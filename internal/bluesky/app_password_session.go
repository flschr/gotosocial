// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
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
		_ = deleteAppPasswordSession(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, fmt.Errorf("parse Bluesky session DID: %w", err)
	}
	if accountDID.String() != did {
		_ = deleteAppPasswordSession(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, ErrIdentityMismatch
	}
	if response.AccessJWT == "" || response.RefreshJWT == "" {
		_ = deleteAppPasswordSession(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, errors.New("Bluesky returned an incomplete app password session")
	}
	if err := validateAppPasswordAccessToken(response.AccessJWT); err != nil {
		_ = deleteAppPasswordSession(ctx, state, pdsURL, response.RefreshJWT)
		return atclient.PasswordSessionData{}, err
	}
	return atclient.PasswordSessionData{
		AccessToken: response.AccessJWT, RefreshToken: response.RefreshJWT,
		AccountDID: accountDID, Host: strings.TrimRight(pdsURL, "/"),
	}, nil
}

func logoutAppPassword(ctx context.Context, state *state.State, connection *gtsmodel.BlueskyConnection) error {
	credentials, err := decodeAppPassword(connection.AccountID, connection.AppPasswordData)
	if err != nil {
		return err
	}
	return deleteAppPasswordSession(ctx, state, credentials.Session.Host, credentials.Session.RefreshToken)
}

func deleteAppPasswordSession(ctx context.Context, state *state.State, host, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(host, "/")+"/xrpc/com.atproto.server.deleteSession", nil)
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
