// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/bluesky-social/indigo/atproto/atcrypto"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

var (
	ErrIdentityMismatch   = errors.New("Bluesky identity does not match saved account")
	ErrCredentialsChanged = errors.New("Bluesky credentials changed concurrently")
)

type OAuthStore struct {
	db                      db.DB
	crypter                 *Crypter
	accountID               string
	mu                      sync.Mutex
	deferSessionPersistence bool
	expectedSessionID       string
	expectedData            []byte
	expectedAccessToken     string
	expectedRefreshToken    string
	discardSession          func(context.Context, oauth.ClientSessionData) error
}

// DeferSessionPersistence keeps ProcessCallback from activating a reconnect
// before the caller has completed identity and PDS validation.
func (s *OAuthStore) DeferSessionPersistence() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deferSessionPersistence = true
}

func (s *OAuthStore) PersistSession(ctx context.Context, session oauth.ClientSessionData) error {
	s.mu.Lock()
	s.deferSessionPersistence = false
	s.mu.Unlock()
	return s.SaveSession(ctx, session)
}

func NewOAuthStore(database db.DB, crypter *Crypter, accountID string, discardSession func(context.Context, oauth.ClientSessionData) error) *OAuthStore {
	return &OAuthStore{db: database, crypter: crypter, accountID: accountID, discardSession: discardSession}
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
	s.mu.Lock()
	s.expectedSessionID = connection.OAuthSessionID
	s.expectedData = append(s.expectedData[:0], connection.OAuthData...)
	s.expectedAccessToken = session.AccessToken
	s.expectedRefreshToken = session.RefreshToken
	s.mu.Unlock()
	return &session, nil
}

func (s *OAuthStore) SaveSession(ctx context.Context, session oauth.ClientSessionData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	connection, err := s.db.GetBlueskyConnectionByAccountID(ctx, s.accountID)
	if errors.Is(err, db.ErrNoEntries) {
		if s.expectedSessionID == session.SessionID {
			s.discardRotatedSession(ctx, session)
			return ErrCredentialsChanged
		}
		// ProcessCallback returns this session to the caller, which creates the
		// connection atomically after identity verification.
		return nil
	}
	if err != nil {
		s.discardRotatedSession(ctx, session)
		return err
	}
	if connection.DID != session.AccountDID.String() {
		s.discardRotatedSession(ctx, session)
		return ErrIdentityMismatch
	}
	if s.deferSessionPersistence {
		return nil
	}
	if len(connection.AppPasswordData) != 0 || (connection.OAuthSessionID != "" && connection.OAuthSessionID != session.SessionID) {
		s.discardRotatedSession(ctx, session)
		return ErrCredentialsChanged
	}
	expectedSessionID := connection.OAuthSessionID
	expectedData := connection.OAuthData
	if s.expectedSessionID == session.SessionID {
		expectedSessionID = s.expectedSessionID
		expectedData = s.expectedData
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		s.discardRotatedSession(ctx, session)
		return fmt.Errorf("encode Bluesky OAuth session: %w", err)
	}
	encrypted, err := s.crypter.Encrypt(encoded, s.associatedData(session.SessionID))
	if err != nil {
		s.discardRotatedSession(ctx, session)
		return err
	}
	updated, err := s.db.UpdateBlueskyOAuthSession(ctx, s.accountID, expectedSessionID, expectedData, session.SessionID, encrypted)
	if err != nil {
		s.discardRotatedSession(ctx, session)
		return err
	}
	if !updated {
		s.discardRotatedSession(ctx, session)
		return ErrCredentialsChanged
	}
	s.expectedSessionID = session.SessionID
	s.expectedData = append(s.expectedData[:0], encrypted...)
	s.expectedAccessToken = session.AccessToken
	s.expectedRefreshToken = session.RefreshToken
	return nil
}

func (s *OAuthStore) discardRotatedSession(ctx context.Context, session oauth.ClientSessionData) {
	if s.discardSession == nil || s.expectedSessionID != session.SessionID ||
		(s.expectedAccessToken == session.AccessToken && s.expectedRefreshToken == session.RefreshToken) {
		return
	}
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	_ = s.discardSession(cleanupCtx, session)
}

type sessionRevoker interface {
	RevokeSession(context.Context) error
}

func revokeSessionDetached(ctx context.Context, session sessionRevoker) {
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	_ = session.RevokeSession(cleanupCtx)
}

// RevokeOAuthSession revokes a callback session that was deliberately not
// persisted, for example after an identity or concurrent-connection check.
func RevokeOAuthSession(ctx context.Context, app *oauth.ClientApp, data oauth.ClientSessionData) error {
	if data.AuthServerRevocationEndpoint == "" {
		return nil
	}
	privateKey, err := atcrypto.ParsePrivateMultibase(data.DPoPPrivateKeyMultibase)
	if err != nil {
		return err
	}
	session := &oauth.ClientSession{
		Client: app.Client, Config: app.Config, Data: &data, DPoPPrivateKey: privateKey,
	}
	return session.RevokeSession(ctx)
}

// RevokeOAuthSessionDetached revokes an unpersisted session even if the
// originating request has already been canceled.
func RevokeOAuthSessionDetached(ctx context.Context, app *oauth.ClientApp, data oauth.ClientSessionData) error {
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	return RevokeOAuthSession(cleanupCtx, app, data)
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
