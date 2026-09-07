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
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type refreshSessionResponse struct {
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
	DID        string `json:"did"`
}

var ErrRequestNotReplayable = errors.New("Bluesky request body cannot be replayed after session refresh")

const appPasswordCleanupTimeout = 10 * time.Second

type appPasswordAuth struct {
	mu       sync.Mutex
	session  atclient.PasswordSessionData
	recreate func(context.Context) (atclient.PasswordSessionData, error)
	persist  func(context.Context, atclient.PasswordSessionData) error
	discard  func(context.Context, atclient.PasswordSessionData) error
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
	if refreshErr != nil && isPasswordTokenError(refreshErr) {
		session, refreshErr = a.recreate(request.Context())
	}
	if refreshErr != nil {
		if IsAppPasswordRejected(refreshErr) {
			return nil, &ConnectionError{Code: ErrorCodeAuth, Err: errors.New("saved Bluesky app password is no longer valid")}
		}
		return nil, refreshErr
	}
	if err := a.persist(request.Context(), session); err != nil {
		a.discardUnpersisted(request.Context(), session)
		var connectionErr *ConnectionError
		if errors.As(err, &connectionErr) {
			return nil, err
		}
		return nil, &ConnectionError{Code: ErrorCodeData, Err: err}
	}
	a.session = session

	if request.Body != nil && request.GetBody == nil {
		return nil, &ConnectionError{Code: ErrorCodeRemote, Err: ErrRequestNotReplayable}
	}
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

func (a *appPasswordAuth) discardUnpersisted(ctx context.Context, session atclient.PasswordSessionData) {
	if a.discard == nil {
		return
	}
	cleanupCtx, cancel := appPasswordCleanupContext(ctx)
	defer cancel()
	_ = a.discard(cleanupCtx, session)
}

func appPasswordCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), appPasswordCleanupTimeout)
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

func persistAppPasswordSession(ctx context.Context, state *state.State, accountID, password string, expected []byte, session atclient.PasswordSessionData) ([]byte, error) {
	encrypted, err := encodeAppPassword(accountID, password, session)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
	updated, err := state.DB.UpdateBlueskyAppPasswordData(ctx, accountID, expected, encrypted)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeData, Err: err}
	}
	if !updated {
		return nil, &ConnectionError{Code: ErrorCodeData, Err: ErrCredentialsChanged}
	}
	return encrypted, nil
}

func isPasswordTokenError(err error) bool {
	var apiErr *atclient.APIError
	return errors.As(err, &apiErr) && (apiErr.Name == "ExpiredToken" || apiErr.Name == "InvalidToken")
}

// IsAppPasswordRejected distinguishes terminal credential failures from
// timeouts, rate limits, and provider 5xx errors that should be retried.
func IsAppPasswordRejected(err error) bool {
	if errors.Is(err, ErrNotAppPassword) || errors.Is(err, ErrPrivilegedAppPassword) {
		return true
	}
	var apiErr *atclient.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return (apiErr.StatusCode == http.StatusUnauthorized &&
		(apiErr.Name == "" || apiErr.Name == "AuthenticationRequired" || apiErr.Name == "InvalidLogin")) ||
		apiErr.Name == "InvalidLogin" ||
		apiErr.Name == "AuthFactorTokenRequired"
}

func IsAppPasswordAccountUnavailable(err error) bool {
	var apiErr *atclient.APIError
	return errors.As(err, &apiErr) && (apiErr.Name == "AccountTakedown" || apiErr.Name == "AccountSuspended")
}
