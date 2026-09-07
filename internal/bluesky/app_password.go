// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

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

type createSessionRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type createSessionResponse struct {
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
	DID        string `json:"did"`
}

type refreshSessionResponse struct {
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
	DID        string `json:"did"`
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
	return encodeAppPassword(accountID, password, session)
}

// ActivateAppPassword atomically switches the saved connection to app-password
// authentication after the password has already been verified and encrypted.
func ActivateAppPassword(ctx context.Context, state *state.State, candidate *gtsmodel.BlueskyConnection, encrypted []byte) (*gtsmodel.BlueskyConnection, error) {
	defer lockAccount(candidate.AccountID)()
	existing, err := state.DB.GetBlueskyConnectionByAccountID(ctx, candidate.AccountID)
	if errors.Is(err, db.ErrNoEntries) {
		candidate.AppPasswordData = encrypted
		if err := state.DB.PutBlueskyConnection(ctx, candidate); err != nil {
			return nil, err
		}
		return candidate, nil
	}
	if err != nil {
		return nil, err
	}
	if existing.DID != candidate.DID {
		return nil, ErrIdentityMismatch
	}
	existing.Handle = candidate.Handle
	existing.PDSURL = candidate.PDSURL
	existing.AppPasswordData = encrypted
	existing.OAuthSessionID = ""
	existing.OAuthData = nil
	existing.LastSyncError = ""
	existing.LastSyncErrorCode = ""
	if err := state.DB.UpdateBlueskyConnection(ctx, existing,
		"handle", "pds_url", "app_password_data", "oauth_session_id", "oauth_data", "last_sync_error", "last_sync_error_code",
	); err != nil {
		return nil, err
	}
	return existing, nil
}

func createAppPasswordSession(ctx context.Context, state *state.State, pdsURL, did, password string) (atclient.PasswordSessionData, error) {
	client := newATClient(state, pdsURL)
	client.Headers.Set("User-Agent", "GoToSocial Plus")
	var response createSessionResponse
	if err := client.Post(ctx, syntax.NSID("com.atproto.server.createSession"), &createSessionRequest{
		Identifier: did,
		Password:   password,
	}, &response); err != nil {
		return atclient.PasswordSessionData{}, err
	}
	accountDID, err := syntax.ParseDID(response.DID)
	if err != nil {
		return atclient.PasswordSessionData{}, fmt.Errorf("parse Bluesky session DID: %w", err)
	}
	if accountDID.String() != did {
		return atclient.PasswordSessionData{}, ErrIdentityMismatch
	}
	if response.AccessJWT == "" || response.RefreshJWT == "" {
		return atclient.PasswordSessionData{}, errors.New("Bluesky returned an incomplete app password session")
	}
	return atclient.PasswordSessionData{
		AccessToken: response.AccessJWT, RefreshToken: response.RefreshJWT,
		AccountDID: accountDID, Host: strings.TrimRight(pdsURL, "/"),
	}, nil
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
	client.Auth = &appPasswordAuth{
		session: credentials.Session,
		recreate: func(ctx context.Context) (atclient.PasswordSessionData, error) {
			return createAppPasswordSession(ctx, state, credentials.Session.Host, credentials.Session.AccountDID.String(), credentials.Password)
		},
		persist: func(ctx context.Context, session atclient.PasswordSessionData) error {
			return persistAppPasswordSession(ctx, state, connection.AccountID, credentials.Password, session)
		},
	}
	client.AccountDID = &credentials.Session.AccountDID
	client.Headers.Set("User-Agent", "GoToSocial Plus")
	return client, nil
}

func logoutAppPassword(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	credentials, err := decodeAppPassword(connection.AccountID, connection.AppPasswordData)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, credentials.Session.Host+"/xrpc/com.atproto.server.deleteSession", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+credentials.Session.RefreshToken)
	req.Header.Set("User-Agent", "GoToSocial Plus")
	response, err := protectedHTTPClient(state).Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &atclient.APIError{StatusCode: response.StatusCode}
	}
	return nil
}

type appPasswordAuth struct {
	mu       sync.Mutex
	session  atclient.PasswordSessionData
	recreate func(context.Context) (atclient.PasswordSessionData, error)
	persist  func(context.Context, atclient.PasswordSessionData) error
}

func (a *appPasswordAuth) DoWithAuth(client *http.Client, request *http.Request, _ syntax.NSID) (*http.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	request.Header.Set("Authorization", "Bearer "+a.session.AccessToken)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	authFailure, parsedErr := passwordAuthFailure(response)
	if !authFailure {
		if parsedErr != nil {
			return nil, parsedErr
		}
		return response, nil
	}

	session, refreshErr := a.refresh(request.Context(), client)
	recreated := false
	if refreshErr != nil && isPasswordTokenError(refreshErr) {
		recreated = true
		session, refreshErr = a.recreate(request.Context())
	}
	if refreshErr != nil {
		if recreated || isPasswordTokenError(refreshErr) {
			return nil, &ConnectionError{Code: ErrorCodeAuth, Err: errors.New("saved Bluesky app password is no longer valid")}
		}
		return nil, refreshErr
	}
	if err := a.persist(request.Context(), session); err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
	a.session = session

	retry := request.Clone(request.Context())
	if request.GetBody != nil {
		retry.Body, err = request.GetBody()
		if err != nil {
			return nil, fmt.Errorf("retry Bluesky request: %w", err)
		}
	}
	retry.Header.Set("Authorization", "Bearer "+a.session.AccessToken)
	return client.Do(retry)
}

func passwordAuthFailure(response *http.Response) (bool, error) {
	if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusUnauthorized {
		return false, nil
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		return false, nil
	}
	defer response.Body.Close()
	var body atclient.ErrorBody
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return false, &atclient.APIError{StatusCode: response.StatusCode}
	}
	apiErr := body.APIError(response.StatusCode)
	if body.Name == "ExpiredToken" || body.Name == "InvalidToken" {
		return true, apiErr
	}
	return false, apiErr
}

func (a *appPasswordAuth) refresh(ctx context.Context, client *http.Client) (atclient.PasswordSessionData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.session.Host+"/xrpc/com.atproto.server.refreshSession", nil)
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.session.RefreshToken)
	req.Header.Set("User-Agent", "GoToSocial Plus")
	response, err := client.Do(req)
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var body atclient.ErrorBody
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			return atclient.PasswordSessionData{}, &atclient.APIError{StatusCode: response.StatusCode}
		}
		return atclient.PasswordSessionData{}, body.APIError(response.StatusCode)
	}
	var body refreshSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return atclient.PasswordSessionData{}, err
	}
	if body.DID != "" && body.DID != a.session.AccountDID.String() {
		return atclient.PasswordSessionData{}, ErrIdentityMismatch
	}
	if body.AccessJWT == "" || body.RefreshJWT == "" {
		return atclient.PasswordSessionData{}, errors.New("Bluesky returned an incomplete refreshed session")
	}
	return atclient.PasswordSessionData{
		AccessToken: body.AccessJWT, RefreshToken: body.RefreshJWT,
		AccountDID: a.session.AccountDID, Host: a.session.Host,
	}, nil
}

func persistAppPasswordSession(ctx context.Context, state *state.State, accountID, password string, session atclient.PasswordSessionData) error {
	connection, err := state.DB.GetBlueskyConnectionByAccountID(ctx, accountID)
	if err != nil {
		return err
	}
	if connection.AuthMethod() != "app_password" || connection.DID != session.AccountDID.String() {
		return db.ErrNoEntries
	}
	encrypted, err := encodeAppPassword(accountID, password, session)
	if err != nil {
		return err
	}
	connection.AppPasswordData = encrypted
	return state.DB.UpdateBlueskyConnection(ctx, connection, "app_password_data")
}

func isPasswordTokenError(err error) bool {
	var apiErr *atclient.APIError
	return errors.As(err, &apiErr) && (apiErr.Name == "ExpiredToken" || apiErr.Name == "InvalidToken")
}
