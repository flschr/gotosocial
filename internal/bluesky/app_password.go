// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type appPasswordCredentials struct {
	Password string                       `json:"password"`
	Session  atclient.PasswordSessionData `json:"session"`
}

func appPasswordAssociatedData(accountID string) []byte {
	return []byte(accountID + "/app-password")
}

func encodeAppPassword(accountID, password string, session atclient.PasswordSessionData) ([]byte, error) {
	crypter, err := NewCrypter(config.GetBlueskyOAuthEncryptionKey())
	if err != nil {
		return nil, err
	}
	plaintext, err := json.Marshal(appPasswordCredentials{Password: password, Session: session})
	if err != nil {
		return nil, fmt.Errorf("encode Bluesky app password session: %w", err)
	}
	return crypter.Encrypt(plaintext, appPasswordAssociatedData(accountID))
}

func decodeAppPassword(accountID string, encrypted []byte) (*appPasswordCredentials, error) {
	crypter, err := NewCrypter(config.GetBlueskyOAuthEncryptionKey())
	if err != nil {
		return nil, err
	}
	plaintext, err := crypter.Decrypt(encrypted, appPasswordAssociatedData(accountID))
	if err != nil {
		return nil, err
	}
	credentials := new(appPasswordCredentials)
	if err := json.Unmarshal(plaintext, credentials); err != nil {
		return nil, fmt.Errorf("decode Bluesky app password session: %w", err)
	}
	if credentials.Password == "" || credentials.Session.AccessToken == "" || credentials.Session.RefreshToken == "" {
		return nil, errors.New("stored Bluesky app password session is incomplete")
	}
	return credentials, nil
}

// CreateAppPasswordData verifies an app password against the identity's PDS
// and returns only an encrypted credential bundle suitable for persistence.
func CreateAppPasswordData(ctx context.Context, state *state.State, accountID, pdsURL, did, password string) ([]byte, error) {
	session, err := createAppPasswordSession(ctx, state, pdsURL, did, password)
	if err != nil {
		return nil, err
	}
	encrypted, err := encodeAppPassword(accountID, password, session)
	if err != nil {
		_ = deleteAppPasswordSession(ctx, state, session.Host, session.RefreshToken)
		return nil, err
	}
	return encrypted, nil
}

// ActivateAppPassword atomically switches the saved connection to app-password
// authentication after the password has already been verified and encrypted.
func ActivateAppPassword(ctx context.Context, state *state.State, candidate *gtsmodel.BlueskyConnection, encrypted []byte) (*gtsmodel.BlueskyConnection, error) {
	defer lockAccount(candidate.AccountID)()
	activated := false
	defer func() {
		if !activated {
			cleanupCtx, cancel := appPasswordCleanupContext(ctx)
			defer cancel()
			_ = revokeAppPasswordData(cleanupCtx, state, candidate.AccountID, encrypted)
		}
	}()
	for range 3 {
		existing, err := state.DB.GetBlueskyConnectionByAccountID(ctx, candidate.AccountID)
		if errors.Is(err, db.ErrNoEntries) {
			candidate.AppPasswordData = encrypted
			if putErr := state.DB.PutBlueskyConnection(ctx, candidate); putErr == nil {
				activated = true
				return candidate, nil
			} else if _, getErr := state.DB.GetBlueskyConnectionByAccountID(ctx, candidate.AccountID); getErr != nil {
				return nil, putErr
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if existing.DID != candidate.DID {
			return nil, ErrIdentityMismatch
		}
		oldAppPasswordData := bytes.Clone(existing.AppPasswordData)
		oldOAuthData := bytes.Clone(existing.OAuthData)
		var oldOAuthSession interface{ RevokeSession(context.Context) error }
		if existing.OAuthSessionID != "" {
			if did, parseErr := syntax.ParseDID(existing.DID); parseErr == nil {
				if app, _, appErr := NewOAuthClient(state, existing.AccountID); appErr == nil {
					if session, resumeErr := app.ResumeSession(ctx, did, existing.OAuthSessionID); resumeErr == nil {
						oldOAuthSession = session
					}
				}
			}
		}
		existing.Handle = candidate.Handle
		existing.PDSURL = candidate.PDSURL
		existing.AppPasswordData = encrypted
		updated, err := state.DB.ActivateBlueskyAppPassword(ctx, existing, existing.OAuthSessionID, oldOAuthData, oldAppPasswordData)
		if err != nil {
			return nil, err
		}
		if !updated {
			continue
		}
		existing.OAuthSessionID = ""
		existing.OAuthData = nil
		existing.LastSyncError = ""
		existing.LastSyncErrorCode = ""
		if len(oldAppPasswordData) != 0 {
			old := *existing
			old.AppPasswordData = oldAppPasswordData
			_ = logoutAppPassword(ctx, state, &old)
		}
		if oldOAuthSession != nil {
			_ = oldOAuthSession.RevokeSession(ctx)
		}
		activated = true
		return existing, nil
	}
	return nil, ErrCredentialsChanged
}

func newAppPasswordClient(state *state.State, connection *gtsmodel.BlueskyConnection) (*atclient.APIClient, error) {
	credentials, err := decodeAppPassword(connection.AccountID, connection.AppPasswordData)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
	if credentials.Session.AccountDID.String() != connection.DID {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: ErrIdentityMismatch}
	}
	client := newATClient(state, credentials.Session.Host)
	expectedData := bytes.Clone(connection.AppPasswordData)
	client.Auth = &appPasswordAuth{
		session: credentials.Session,
		recreate: func(ctx context.Context) (atclient.PasswordSessionData, error) {
			return createAppPasswordSession(ctx, state, credentials.Session.Host, credentials.Session.AccountDID.String(), credentials.Password)
		},
		persist: func(ctx context.Context, session atclient.PasswordSessionData) error {
			replacement, err := persistAppPasswordSession(ctx, state, connection.AccountID, credentials.Password, expectedData, session)
			if err == nil {
				expectedData = replacement
			}
			return err
		},
		discard: func(ctx context.Context, session atclient.PasswordSessionData) error {
			return deleteAppPasswordSession(ctx, state, session.Host, session.RefreshToken)
		},
	}
	client.AccountDID = &credentials.Session.AccountDID
	client.Headers.Set("User-Agent", "GoToSocial Plus")
	return client, nil
}
