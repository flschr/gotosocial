// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

var ErrIdentityMismatch = errors.New("Bluesky identity does not match saved account")

type OAuthStore struct {
	db                      db.DB
	crypter                 *Crypter
	accountID               string
	deferSessionPersistence bool
}

// DeferSessionPersistence keeps ProcessCallback from activating a reconnect
// before the caller has completed identity and PDS validation.
func (s *OAuthStore) DeferSessionPersistence() {
	s.deferSessionPersistence = true
}

func (s *OAuthStore) PersistSession(ctx context.Context, session oauth.ClientSessionData) error {
	s.deferSessionPersistence = false
	return s.SaveSession(ctx, session)
}

func NewOAuthStore(database db.DB, crypter *Crypter, accountID string) *OAuthStore {
	return &OAuthStore{db: database, crypter: crypter, accountID: accountID}
}

func (s *OAuthStore) associatedData(sessionID string) []byte {
	return []byte(s.accountID + "/" + sessionID)
}

func (s *OAuthStore) GetSession(ctx context.Context, _ syntax.DID, sessionID string) (*oauth.ClientSessionData, error) {
	connection, err := s.db.GetBlueskyConnectionByAccountID(ctx, s.accountID)
	if err != nil {
		return nil, err
	}
	if connection.OAuthSessionID != sessionID || len(connection.OAuthData) == 0 {
		return nil, db.ErrNoEntries
	}
	plaintext, err := s.crypter.Decrypt(connection.OAuthData, s.associatedData(sessionID))
	if err != nil {
		return nil, err
	}
	var session oauth.ClientSessionData
	if err := json.Unmarshal(plaintext, &session); err != nil {
		return nil, fmt.Errorf("decode Bluesky OAuth session: %w", err)
	}
	return &session, nil
}

func (s *OAuthStore) SaveSession(ctx context.Context, session oauth.ClientSessionData) error {
	connection, err := s.db.GetBlueskyConnectionByAccountID(ctx, s.accountID)
	if errors.Is(err, db.ErrNoEntries) {
		// ProcessCallback returns this session to the caller, which creates the
		// connection atomically after identity verification.
		return nil
	}
	if err != nil {
		return err
	}
	if connection.DID != session.AccountDID.String() {
		return ErrIdentityMismatch
	}
	if s.deferSessionPersistence {
		return nil
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode Bluesky OAuth session: %w", err)
	}
	encrypted, err := s.crypter.Encrypt(encoded, s.associatedData(session.SessionID))
	if err != nil {
		return err
	}
	connection.OAuthSessionID = session.SessionID
	connection.OAuthData = encrypted
	return s.db.UpdateBlueskyConnection(ctx, connection, "oauth_session_id", "oauth_data")
}

func (s *OAuthStore) DeleteSession(ctx context.Context, _ syntax.DID, sessionID string) error {
	connection, err := s.db.GetBlueskyConnectionByAccountID(ctx, s.accountID)
	if errors.Is(err, db.ErrNoEntries) {
		return nil
	}
	if err != nil {
		return err
	}
	if connection.OAuthSessionID != sessionID {
		return nil
	}
	connection.OAuthSessionID = ""
	connection.OAuthData = nil
	return s.db.UpdateBlueskyConnection(ctx, connection, "oauth_session_id", "oauth_data")
}

func (s *OAuthStore) GetAuthRequestInfo(ctx context.Context, state string) (*oauth.AuthRequestData, error) {
	stored, err := s.db.GetBlueskyOAuthState(ctx, state)
	if err != nil {
		return nil, err
	}
	if stored.AccountID != s.accountID {
		return nil, db.ErrNoEntries
	}
	plaintext, err := s.crypter.Decrypt(stored.EncryptedData, s.associatedData(state))
	if err != nil {
		return nil, err
	}
	var request oauth.AuthRequestData
	if err := json.Unmarshal(plaintext, &request); err != nil {
		return nil, fmt.Errorf("decode Bluesky OAuth request: %w", err)
	}
	return &request, nil
}

func (s *OAuthStore) SaveAuthRequestInfo(ctx context.Context, request oauth.AuthRequestData) error {
	encoded, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode Bluesky OAuth request: %w", err)
	}
	encrypted, err := s.crypter.Encrypt(encoded, s.associatedData(request.State))
	if err != nil {
		return err
	}
	return s.db.PutBlueskyOAuthState(ctx, &gtsmodel.BlueskyOAuthState{
		ID:            id.NewULID(),
		AccountID:     s.accountID,
		State:         request.State,
		EncryptedData: encrypted,
	})
}

func (s *OAuthStore) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	stored, err := s.db.GetBlueskyOAuthState(ctx, state)
	if errors.Is(err, db.ErrNoEntries) {
		return nil
	}
	if err != nil {
		return err
	}
	if stored.AccountID != s.accountID {
		return db.ErrNoEntries
	}
	return s.db.DeleteBlueskyOAuthState(ctx, state)
}
