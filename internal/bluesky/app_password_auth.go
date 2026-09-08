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
	"net/url"
	"strings"
	"sync"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
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

const credentialCleanupTimeout = 10 * time.Second

type appPasswordAuth struct {
	mu          sync.Mutex
	session     atclient.PasswordSessionData
	requestHost string
	recreate    func(context.Context) (atclient.PasswordSessionData, error)
	persist     func(context.Context, atclient.PasswordSessionData) error
	discard     func(context.Context, atclient.PasswordSessionData) error
}

func (a *appPasswordAuth) DoWithAuth(client *http.Client, request *http.Request, method syntax.NSID) (*http.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	previousHost := a.session.Host
	requestHost := a.requestHost
	if requestHost == "" {
		requestHost = previousHost
	}
	request = request.WithContext(gtscontext.SetNoRedirect(request.Context()))
	if err := retargetAppPasswordRequest(request, requestHost, a.session.Host, method); err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
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
	if err := retargetAppPasswordRequest(retry, previousHost, a.session.Host, method); err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
	return client.Do(retry)
}

func retargetAppPasswordRequest(request *http.Request, previousHost, replacementHost string, method syntax.NSID) error {
	previous, err := url.Parse(previousHost)
	if err != nil {
		return err
	}
	if request.URL.Scheme != previous.Scheme || request.URL.Host != previous.Host {
		return nil
	}
	endpoint, err := appPasswordXRPCURL(replacementHost, string(method))
	if err != nil {
		return err
	}
	replacement, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	request.URL.Scheme = replacement.Scheme
	request.URL.Host = replacement.Host
	request.URL.Path = replacement.Path
	request.URL.RawPath = replacement.RawPath
	return nil
}

func (a *appPasswordAuth) discardUnpersisted(ctx context.Context, session atclient.PasswordSessionData) {
	if a.discard == nil {
		return
	}
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	_ = a.discard(cleanupCtx, session)
}

func credentialCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), credentialCleanupTimeout)
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
	endpoint, err := appPasswordXRPCURL(a.session.Host, "com.atproto.server.refreshSession")
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	req, err := http.NewRequestWithContext(gtscontext.SetNoRedirect(ctx), http.MethodPost, endpoint, nil)
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
	rotated := atclient.PasswordSessionData{
		AccessToken: body.AccessJWT, RefreshToken: body.RefreshJWT,
		AccountDID: a.session.AccountDID, Host: a.session.Host,
	}
	if body.DID != "" && body.DID != a.session.AccountDID.String() {
		a.discardUnpersisted(ctx, rotated)
		return atclient.PasswordSessionData{}, ErrIdentityMismatch
	}
	if body.AccessJWT == "" || body.RefreshJWT == "" {
		a.discardUnpersisted(ctx, rotated)
		return atclient.PasswordSessionData{}, errors.New("Bluesky returned an incomplete refreshed session")
	}
	if err := validateAppPasswordAccessToken(body.AccessJWT); err != nil {
		a.discardUnpersisted(ctx, rotated)
		return atclient.PasswordSessionData{}, err
	}
	return rotated, nil
}

func persistAppPasswordSession(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection, password string, expected []byte, session atclient.PasswordSessionData) ([]byte, error) {
	encrypted, err := encodeAppPassword(connection.AccountID, password, session)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeConfiguration, Err: err}
	}
	updated, err := state.DB.UpdateBlueskyAppPasswordData(ctx, connection, expected, encrypted, session.Host)
	if err != nil {
		return nil, &ConnectionError{Code: ErrorCodeData, Err: err}
	}
	if !updated {
		return nil, &ConnectionError{Code: ErrorCodeData, Err: ErrCredentialsChanged}
	}
	connection.PDSURL = session.Host
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
