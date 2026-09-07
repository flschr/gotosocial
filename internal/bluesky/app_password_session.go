// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"code.superseriousbusiness.org/gotosocial/internal/gtscontext"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

type createSessionRequest struct {
	Identifier string `json:"identifier"`
	Password   string `json:"password"`
}

type createSessionResponse struct {
	AccessJWT  string `json:"accessJwt"`
	RefreshJWT string `json:"refreshJwt"`
	DID        string `json:"did"`
}

type accessTokenClaims struct {
	Scope string `json:"scope"`
}

var (
	ErrNotAppPassword        = errors.New("Bluesky credential is not an app password")
	ErrPrivilegedAppPassword = errors.New("Bluesky app password grants privileged access")
)

func revokeAppPasswordData(ctx context.Context, state *state.State, accountID string, encrypted []byte) error {
	credentials, err := decodeAppPassword(accountID, encrypted)
	if err != nil {
		return err
	}
	return deleteAppPasswordSession(ctx, state, credentials.Session.Host, credentials.Session.RefreshToken)
}

func revokeAppPasswordDataDetached(ctx context.Context, state *state.State, accountID string, encrypted []byte) {
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	_ = revokeAppPasswordData(cleanupCtx, state, accountID, encrypted)
}

func deleteAppPasswordSessionDetached(ctx context.Context, state *state.State, host, refreshToken string) {
	cleanupCtx, cancel := credentialCleanupContext(ctx)
	defer cancel()
	_ = deleteAppPasswordSession(cleanupCtx, state, host, refreshToken)
}

func createAppPasswordSession(ctx context.Context, state *state.State, pdsURL, did, password string) (atclient.PasswordSessionData, error) {
	body, err := json.Marshal(&createSessionRequest{Identifier: did, Password: password})
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	endpoint, err := appPasswordXRPCURL(pdsURL, "com.atproto.server.createSession")
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	request, err := http.NewRequestWithContext(gtscontext.SetNoRedirect(ctx), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "GoToSocial Plus")
	httpResponse, err := protectedHTTPClient(state).Do(request)
	if err != nil {
		return atclient.PasswordSessionData{}, err
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		var errorBody atclient.ErrorBody
		if err := json.NewDecoder(httpResponse.Body).Decode(&errorBody); err != nil {
			return atclient.PasswordSessionData{}, &atclient.APIError{StatusCode: httpResponse.StatusCode}
		}
		return atclient.PasswordSessionData{}, errorBody.APIError(httpResponse.StatusCode)
	}
	var response createSessionResponse
	if err := json.NewDecoder(httpResponse.Body).Decode(&response); err != nil {
		return atclient.PasswordSessionData{}, err
	}
	accountDID, err := syntax.ParseDID(response.DID)
	if err != nil {
		deleteAppPasswordSessionDetached(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, fmt.Errorf("parse Bluesky session DID: %w", err)
	}
	if accountDID.String() != did {
		deleteAppPasswordSessionDetached(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, ErrIdentityMismatch
	}
	if response.AccessJWT == "" || response.RefreshJWT == "" {
		deleteAppPasswordSessionDetached(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, errors.New("Bluesky returned an incomplete app password session")
	}
	if err := validateAppPasswordAccessToken(response.AccessJWT); err != nil {
		deleteAppPasswordSessionDetached(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, err
	}
	return atclient.PasswordSessionData{
		AccessToken: response.AccessJWT, RefreshToken: response.RefreshJWT,
		AccountDID: accountDID, Host: strings.TrimRight(pdsURL, "/"),
	}, nil
}

func deleteAppPasswordSession(ctx context.Context, state *state.State, host, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	endpoint, err := appPasswordXRPCURL(host, "com.atproto.server.deleteSession")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(gtscontext.SetNoRedirect(ctx), http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+refreshToken)
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

func appPasswordXRPCURL(host, endpoint string) (string, error) {
	u, err := url.Parse(host)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" || u.User != nil {
		return "", errors.New("invalid Bluesky PDS URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
		address, addressErr := netip.ParseAddr(host)
		loopback := host == "localhost" || (addressErr == nil && address.IsLoopback())
		if !strings.EqualFold(u.Scheme, "http") || !loopback {
			return "", errors.New("Bluesky PDS URL must use HTTPS")
		}
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/xrpc/" + endpoint
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func validateAppPasswordAccessToken(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("Bluesky returned a malformed app password access token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("Bluesky returned a malformed app password access token")
	}
	var claims accessTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return errors.New("Bluesky returned a malformed app password access token")
	}
	if claims.Scope == "com.atproto.appPassPrivileged" {
		return ErrPrivilegedAppPassword
	}
	if claims.Scope != "com.atproto.appPass" {
		return ErrNotAppPassword
	}
	return nil
}
