// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/httpclient"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/bluesky-social/indigo/atproto/atclient"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/stretchr/testify/require"
)

func useAppPasswordTestKey(t *testing.T) {
	t.Helper()
	previous := config.GetBlueskyOAuthEncryptionKey()
	config.SetBlueskyOAuthEncryptionKey(base64.StdEncoding.EncodeToString(make([]byte, encryptionKeySize)))
	t.Cleanup(func() { config.SetBlueskyOAuthEncryptionKey(previous) })
}

func testPasswordSession(t *testing.T, host, access, refresh string) atclient.PasswordSessionData {
	t.Helper()
	did, err := syntax.ParseDID("did:plc:apppasswordtest")
	require.NoError(t, err)
	return atclient.PasswordSessionData{AccessToken: access, RefreshToken: refresh, AccountDID: did, Host: host}
}

func TestAppPasswordCredentialsAreEncrypted(t *testing.T) {
	useAppPasswordTestKey(t)
	session := testPasswordSession(t, "https://pds.example.test", "access-secret", "refresh-secret")
	encrypted, err := encodeAppPassword("account", "app-password-secret", session)
	require.NoError(t, err)
	require.NotContains(t, string(encrypted), "app-password-secret")
	require.NotContains(t, string(encrypted), "access-secret")

	decoded, err := decodeAppPassword("account", encrypted)
	require.NoError(t, err)
	require.Equal(t, "app-password-secret", decoded.Password)
	require.Equal(t, session, decoded.Session)
	_, err = decodeAppPassword("different-account", encrypted)
	require.Error(t, err)
}

func TestBlueskyConnectionAuthMethod(t *testing.T) {
	require.Equal(t, "", (*gtsmodel.BlueskyConnection)(nil).AuthMethod())
	require.Equal(t, "", new(gtsmodel.BlueskyConnection).AuthMethod())
	require.Equal(t, "oauth", (&gtsmodel.BlueskyConnection{OAuthSessionID: "session", OAuthData: []byte("encrypted")}).AuthMethod())
	require.Equal(t, "app_password", (&gtsmodel.BlueskyConnection{AppPasswordData: []byte("encrypted")}).AuthMethod())
}

func TestCreateAppPasswordSessionUsesResolvedDID(t *testing.T) {
	const password = "abcd-efgh-ijkl-mnop"
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/xrpc/com.atproto.server.createSession", request.URL.Path)
		var body createSessionRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, "did:plc:apppasswordtest", body.Identifier)
		require.Equal(t, password, body.Password)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"accessJwt":"access","refreshJwt":"refresh","did":"did:plc:apppasswordtest"}`))
	}))
	defer server.Close()
	loopback := netip.MustParsePrefix("127.0.0.0/8")
	testState := &state.State{HTTPClient: httpclient.New(httpclient.Config{AllowRanges: []netip.Prefix{loopback}, Timeout: time.Second})}

	session, err := createAppPasswordSession(t.Context(), testState, server.URL, "did:plc:apppasswordtest", password)
	require.NoError(t, err)
	require.Equal(t, "access", session.AccessToken)
	require.Equal(t, "refresh", session.RefreshToken)
	require.Equal(t, server.URL, session.Host)
}

func TestAppPasswordAuthRefreshesAndPersistsSession(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/xrpc/app.test.endpoint":
			requests++
			response.Header().Set("Content-Type", "application/json")
			if requests == 1 {
				require.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
				response.WriteHeader(http.StatusBadRequest)
				_, _ = response.Write([]byte(`{"error":"ExpiredToken"}`))
				return
			}
			require.Equal(t, "Bearer new-access", request.Header.Get("Authorization"))
			_, _ = response.Write([]byte(`{}`))
		case "/xrpc/com.atproto.server.refreshSession":
			require.Equal(t, "Bearer old-refresh", request.Header.Get("Authorization"))
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"accessJwt":"new-access","refreshJwt":"new-refresh","did":"did:plc:apppasswordtest"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	var persisted atclient.PasswordSessionData
	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		recreate: func(context.Context) (atclient.PasswordSessionData, error) {
			return atclient.PasswordSessionData{}, errors.New("unexpected recreation")
		},
		persist: func(_ context.Context, session atclient.PasswordSessionData) error {
			persisted = session
			return nil
		},
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xrpc/app.test.endpoint", nil)
	require.NoError(t, err)
	response, err := auth.DoWithAuth(server.Client(), request, syntax.NSID("app.test.endpoint"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, "new-access", persisted.AccessToken)
	require.Equal(t, "new-refresh", persisted.RefreshToken)
	require.Equal(t, 2, requests)
}

func TestAppPasswordAuthRecreatesExpiredSession(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/xrpc/app.test.endpoint":
			requests++
			if requests == 1 {
				response.WriteHeader(http.StatusBadRequest)
				_, _ = response.Write([]byte(`{"error":"ExpiredToken"}`))
				return
			}
			require.Equal(t, "Bearer recreated-access", request.Header.Get("Authorization"))
			_, _ = response.Write([]byte(`{}`))
		case "/xrpc/com.atproto.server.refreshSession":
			response.WriteHeader(http.StatusBadRequest)
			_, _ = response.Write([]byte(`{"error":"InvalidToken"}`))
		}
	}))
	defer server.Close()

	recreated := testPasswordSession(t, server.URL, "recreated-access", "recreated-refresh")
	var persisted atclient.PasswordSessionData
	auth := &appPasswordAuth{
		session:  testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		recreate: func(context.Context) (atclient.PasswordSessionData, error) { return recreated, nil },
		persist: func(_ context.Context, session atclient.PasswordSessionData) error {
			persisted = session
			return nil
		},
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xrpc/app.test.endpoint", nil)
	require.NoError(t, err)
	response, err := auth.DoWithAuth(server.Client(), request, syntax.NSID("app.test.endpoint"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, recreated, persisted)
	require.Equal(t, 2, requests)
}

func TestAppPasswordAuthReportsRevokedPasswordWithoutSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusBadRequest)
		if strings.HasSuffix(request.URL.Path, "refreshSession") {
			_, _ = response.Write([]byte(`{"error":"InvalidToken"}`))
			return
		}
		_, _ = response.Write([]byte(`{"error":"ExpiredToken"}`))
	}))
	defer server.Close()

	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		recreate: func(context.Context) (atclient.PasswordSessionData, error) {
			return atclient.PasswordSessionData{}, &atclient.APIError{StatusCode: http.StatusUnauthorized, Name: "InvalidLogin"}
		},
		persist: func(context.Context, atclient.PasswordSessionData) error { return nil },
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xrpc/app.test.endpoint", nil)
	require.NoError(t, err)
	_, err = auth.DoWithAuth(server.Client(), request, syntax.NSID("app.test.endpoint"))
	require.Error(t, err)
	require.Equal(t, ErrorCodeAuth, errorCode(err))
	require.NotContains(t, err.Error(), "password-secret")
	require.Contains(t, err.Error(), "no longer valid")
}
