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
	"code.superseriousbusiness.org/gotosocial/internal/db"
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

type testSessionRevoker func(context.Context) error

func (fn testSessionRevoker) RevokeSession(ctx context.Context) error {
	return fn(ctx)
}

type appPasswordMigrationDB struct {
	db.DB
	update func(context.Context, *gtsmodel.BlueskyConnection, []byte, []byte, string) (bool, error)
}

func (d *appPasswordMigrationDB) UpdateBlueskyAppPasswordData(ctx context.Context, connection *gtsmodel.BlueskyConnection, expected, replacement []byte, replacementPDSURL string) (bool, error) {
	return d.update(ctx, connection, expected, replacement, replacementPDSURL)
}

func testPasswordSession(t *testing.T, host, access, refresh string) atclient.PasswordSessionData {
	t.Helper()
	did, err := syntax.ParseDID("did:plc:apppasswordtest")
	require.NoError(t, err)
	return atclient.PasswordSessionData{AccessToken: access, RefreshToken: refresh, AccountDID: did, Host: host}
}

func testAccessToken(t *testing.T, scope string) string {
	t.Helper()
	payload, err := json.Marshal(accessTokenClaims{Scope: scope})
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
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
		require.Equal(t, "/pds/xrpc/com.atproto.server.createSession", request.URL.Path)
		var body createSessionRequest
		require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
		require.Equal(t, "did:plc:apppasswordtest", body.Identifier)
		require.Equal(t, password, body.Password)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"accessJwt":"` + testAccessToken(t, "com.atproto.appPass") + `","refreshJwt":"refresh","did":"did:plc:apppasswordtest"}`))
	}))
	defer server.Close()
	loopback := netip.MustParsePrefix("127.0.0.0/8")
	testState := &state.State{HTTPClient: httpclient.New(httpclient.Config{AllowRanges: []netip.Prefix{loopback}, Timeout: time.Second})}

	pdsURL := server.URL + "/pds"
	session, err := createAppPasswordSession(t.Context(), testState, pdsURL, "did:plc:apppasswordtest", password)
	require.NoError(t, err)
	require.Equal(t, testAccessToken(t, "com.atproto.appPass"), session.AccessToken)
	require.Equal(t, "refresh", session.RefreshToken)
	require.Equal(t, pdsURL, session.Host)
}

func TestAppPasswordXRPCURLRejectsPlaintextRemotePDS(t *testing.T) {
	_, err := appPasswordXRPCURL("http://pds.example.test", "com.atproto.server.createSession")
	require.EqualError(t, err, "Bluesky PDS URL must use HTTPS")

	endpoint, err := appPasswordXRPCURL("http://127.0.0.1:3000/pds", "com.atproto.server.createSession")
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:3000/pds/xrpc/com.atproto.server.createSession", endpoint)
}

func TestNewAppPasswordClientRejectsStoredPlaintextRemotePDS(t *testing.T) {
	useAppPasswordTestKey(t)
	connection := &gtsmodel.BlueskyConnection{
		ID: "connection", AccountID: "account", DID: "did:plc:apppasswordtest",
	}
	var err error
	connection.AppPasswordData, err = encodeAppPassword(
		connection.AccountID, "app-password",
		testPasswordSession(t, "http://pds.example.test", "access-secret", "refresh-secret"),
	)
	require.NoError(t, err)

	_, err = newAppPasswordClient(new(state.State), connection)
	require.ErrorContains(t, err, "must use HTTPS")
}

func TestNewAppPasswordClientRecreatesSessionOnCurrentPDS(t *testing.T) {
	useAppPasswordTestKey(t)
	const password = "abcd-efgh-ijkl-mnop"
	var did, oldPDS, newPDS string
	requests := make([]string, 0, 5)
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.Method+" "+request.URL.RequestURI())
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/did.json":
			_ = json.NewEncoder(response).Encode(map[string]any{
				"id": did,
				"service": []map[string]string{{
					"id": "#atproto_pds", "type": "AtprotoPersonalDataServer", "serviceEndpoint": newPDS,
				}},
			})
		case "/old-pds/xrpc/app.test.endpoint":
			require.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
			response.WriteHeader(http.StatusUnauthorized)
			_, _ = response.Write([]byte(`{"error":"InvalidToken"}`))
		case "/old-pds/xrpc/com.atproto.server.refreshSession":
			require.Equal(t, "Bearer old-refresh", request.Header.Get("Authorization"))
			response.WriteHeader(http.StatusBadRequest)
			_, _ = response.Write([]byte(`{"error":"InvalidToken"}`))
		case "/old-pds/xrpc/com.atproto.server.createSession":
			t.Fatal("app password was submitted to the stale PDS")
		case "/new-pds/xrpc/com.atproto.server.createSession":
			var body createSessionRequest
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			require.Equal(t, did, body.Identifier)
			require.Equal(t, password, body.Password)
			_ = json.NewEncoder(response).Encode(map[string]string{
				"accessJwt":  testAccessToken(t, "com.atproto.appPass"),
				"refreshJwt": "new-refresh",
				"did":        did,
			})
		case "/new-pds/xrpc/app.test.endpoint":
			require.Contains(t, []string{"cursor=next", "cursor=again"}, request.URL.RawQuery)
			require.Equal(t, "Bearer "+testAccessToken(t, "com.atproto.appPass"), request.Header.Get("Authorization"))
			_, _ = response.Write([]byte(`{}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "https://")
	did = "did:web:" + strings.ReplaceAll(host, ":", "%3A")
	oldPDS = server.URL + "/old-pds"
	newPDS = server.URL + "/new-pds"

	testState := &state.State{HTTPClient: httpclient.New(httpclient.Config{
		AllowRanges:           []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")},
		TLSInsecureSkipVerify: true,
		Timeout:               time.Second,
	})}
	oldSessionDID, err := syntax.ParseDID(did)
	require.NoError(t, err)
	oldSession := atclient.PasswordSessionData{
		AccessToken: "old-access", RefreshToken: "old-refresh", AccountDID: oldSessionDID, Host: oldPDS,
	}
	connection := &gtsmodel.BlueskyConnection{
		ID: "connection", AccountID: "account", DID: did, Handle: "migrated.example", PDSURL: oldPDS,
	}
	connection.AppPasswordData, err = encodeAppPassword(connection.AccountID, password, oldSession)
	require.NoError(t, err)
	expectedData := append([]byte(nil), connection.AppPasswordData...)
	var persisted []byte
	testState.DB = &appPasswordMigrationDB{update: func(_ context.Context, updated *gtsmodel.BlueskyConnection, expected, replacement []byte, replacementPDSURL string) (bool, error) {
		require.Equal(t, expectedData, expected)
		require.Equal(t, oldPDS, updated.PDSURL)
		require.Equal(t, newPDS, replacementPDSURL)
		persisted = append([]byte(nil), replacement...)
		return true, nil
	}}

	client, err := newAppPasswordClient(testState, connection)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, oldPDS+"/xrpc/app.test.endpoint?cursor=next", nil)
	require.NoError(t, err)
	transportClient := &http.Client{Transport: testState.HTTPClient, Timeout: 2 * time.Second}
	response, err := client.Auth.DoWithAuth(transportClient, request, syntax.NSID("app.test.endpoint"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, newPDS, connection.PDSURL)
	require.NotEmpty(t, persisted)
	decoded, err := decodeAppPassword(connection.AccountID, persisted)
	require.NoError(t, err)
	require.Equal(t, newPDS, decoded.Session.Host)
	request, err = http.NewRequestWithContext(t.Context(), http.MethodGet, oldPDS+"/xrpc/app.test.endpoint?cursor=again", nil)
	require.NoError(t, err)
	response, err = client.Auth.DoWithAuth(transportClient, request, syntax.NSID("app.test.endpoint"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, []string{
		"GET /old-pds/xrpc/app.test.endpoint?cursor=next",
		"POST /old-pds/xrpc/com.atproto.server.refreshSession",
		"GET /.well-known/did.json",
		"POST /new-pds/xrpc/com.atproto.server.createSession",
		"GET /new-pds/xrpc/app.test.endpoint?cursor=next",
		"GET /new-pds/xrpc/app.test.endpoint?cursor=again",
	}, requests)
}

func TestCreateAppPasswordSessionDoesNotFollowRedirect(t *testing.T) {
	receivedPassword := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		receivedPassword <- struct{}{}
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Location", target.URL+"/xrpc/com.atproto.server.createSession")
		response.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	testState := &state.State{HTTPClient: httpclient.New(httpclient.Config{
		AllowRanges: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Timeout: time.Second,
	})}

	_, err := createAppPasswordSession(t.Context(), testState, redirector.URL, "did:plc:apppasswordtest", "must-not-leak")
	require.Error(t, err)
	select {
	case <-receivedPassword:
		t.Fatal("app password request followed a redirect")
	default:
	}
}

func TestCreateAppPasswordSessionRejectsMainPasswordScopeAndRevokesSession(t *testing.T) {
	revoked := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/xrpc/com.atproto.server.createSession":
			_, _ = response.Write([]byte(`{"accessJwt":"` + testAccessToken(t, "com.atproto.access") + `","refreshJwt":"main-password-refresh","did":"did:plc:apppasswordtest"}`))
		case "/xrpc/com.atproto.server.deleteSession":
			require.Equal(t, "Bearer main-password-refresh", request.Header.Get("Authorization"))
			revoked = true
			_, _ = response.Write([]byte(`{}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	loopback := netip.MustParsePrefix("127.0.0.0/8")
	testState := &state.State{HTTPClient: httpclient.New(httpclient.Config{AllowRanges: []netip.Prefix{loopback}, Timeout: time.Second})}

	_, err := createAppPasswordSession(t.Context(), testState, server.URL, "did:plc:apppasswordtest", "main-password")
	require.ErrorIs(t, err, ErrNotAppPassword)
	require.True(t, revoked)
}

func TestDeleteAppPasswordSessionDetachedSurvivesRequestCancellation(t *testing.T) {
	revoked := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/pds/xrpc/com.atproto.server.deleteSession", request.URL.Path)
		require.Equal(t, "Bearer cleanup-refresh", request.Header.Get("Authorization"))
		revoked <- struct{}{}
		_, _ = response.Write([]byte(`{}`))
	}))
	defer server.Close()
	testState := &state.State{HTTPClient: httpclient.New(httpclient.Config{
		AllowRanges: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, Timeout: time.Second,
	})}
	requestCtx, cancel := context.WithCancel(t.Context())
	cancel()
	deleteAppPasswordSessionDetached(requestCtx, testState, server.URL+"/pds", "cleanup-refresh")
	select {
	case <-revoked:
	default:
		t.Fatal("rejected session was not revoked")
	}
}

func TestAppPasswordAuthRefreshesAndPersistsSession(t *testing.T) {
	requests := 0
	newAccess := testAccessToken(t, "com.atproto.appPass")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/pds/xrpc/app.test.endpoint":
			requests++
			response.Header().Set("Content-Type", "application/json")
			if requests == 1 {
				require.Equal(t, "Bearer old-access", request.Header.Get("Authorization"))
				response.WriteHeader(http.StatusBadRequest)
				_, _ = response.Write([]byte(`{"error":"ExpiredToken"}`))
				return
			}
			require.Equal(t, "Bearer "+newAccess, request.Header.Get("Authorization"))
			_, _ = response.Write([]byte(`{}`))
		case "/pds/xrpc/com.atproto.server.refreshSession":
			require.Equal(t, "Bearer old-refresh", request.Header.Get("Authorization"))
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"accessJwt":"` + newAccess + `","refreshJwt":"new-refresh","did":"did:plc:apppasswordtest"}`))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	var persisted atclient.PasswordSessionData
	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL+"/pds", "old-access", "old-refresh"),
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
	require.Equal(t, newAccess, persisted.AccessToken)
	require.Equal(t, "new-refresh", persisted.RefreshToken)
	require.Equal(t, 2, requests)
}

func TestAppPasswordAuthRevokesRefreshIdentityMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/xrpc/com.atproto.server.refreshSession", request.URL.Path)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"accessJwt":"` + testAccessToken(t, "com.atproto.appPass") + `","refreshJwt":"rotated-refresh","did":"did:plc:different"}`))
	}))
	defer server.Close()

	var discarded atclient.PasswordSessionData
	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		discard: func(ctx context.Context, session atclient.PasswordSessionData) error {
			require.NoError(t, ctx.Err())
			discarded = session
			return nil
		},
	}
	_, err := auth.refresh(t.Context(), server.Client())
	require.ErrorIs(t, err, ErrIdentityMismatch)
	require.Equal(t, "rotated-refresh", discarded.RefreshToken)
	require.Equal(t, server.URL, discarded.Host)
}

func TestAppPasswordRetargetOnlyAppliesToPDSRequests(t *testing.T) {
	pdsRequest, err := http.NewRequest(http.MethodPost, "https://pds.example.test/xrpc/app.test.endpoint", nil)
	require.NoError(t, err)
	require.NoError(t, retargetAppPasswordRequest(pdsRequest, "https://pds.example.test", "https://pds.example.test/pds", syntax.NSID("app.test.endpoint")))
	require.Equal(t, "/pds/xrpc/app.test.endpoint", pdsRequest.URL.Path)

	appViewRequest, err := http.NewRequest(http.MethodGet, "https://api.bsky.app/xrpc/app.bsky.notification.listNotifications", nil)
	require.NoError(t, err)
	require.NoError(t, retargetAppPasswordRequest(appViewRequest, "https://pds.example.test", "https://pds.example.test/pds", syntax.NSID("app.bsky.notification.listNotifications")))
	require.Equal(t, "/xrpc/app.bsky.notification.listNotifications", appViewRequest.URL.Path)
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

func TestAppPasswordAuthPreservesTransientRecreationError(t *testing.T) {
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

	transient := &atclient.APIError{StatusCode: http.StatusServiceUnavailable, Name: "UpstreamFailure"}
	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		recreate: func(context.Context) (atclient.PasswordSessionData, error) {
			return atclient.PasswordSessionData{}, transient
		},
		persist: func(context.Context, atclient.PasswordSessionData) error { return nil },
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+"/xrpc/app.test.endpoint", nil)
	require.NoError(t, err)
	_, err = auth.DoWithAuth(server.Client(), request, syntax.NSID("app.test.endpoint"))
	require.ErrorIs(t, err, transient)
	require.Equal(t, ErrorCodeRemote, errorCode(err))
}

func TestAppPasswordAuthRevokesSessionThatCannotBePersisted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/xrpc/app.test.endpoint":
			response.WriteHeader(http.StatusBadRequest)
			_, _ = response.Write([]byte(`{"error":"ExpiredToken"}`))
		case "/xrpc/com.atproto.server.refreshSession":
			_, _ = response.Write([]byte(`{"accessJwt":"` + testAccessToken(t, "com.atproto.appPass") + `","refreshJwt":"new-refresh","did":"did:plc:apppasswordtest"}`))
		}
	}))
	defer server.Close()

	requestCtx, cancel := context.WithCancel(t.Context())
	discarded := false
	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		recreate: func(context.Context) (atclient.PasswordSessionData, error) {
			return atclient.PasswordSessionData{}, errors.New("unexpected recreation")
		},
		persist: func(context.Context, atclient.PasswordSessionData) error {
			cancel()
			return &ConnectionError{Code: ErrorCodeData, Err: ErrCredentialsChanged}
		},
		discard: func(ctx context.Context, session atclient.PasswordSessionData) error {
			require.NoError(t, ctx.Err())
			require.Equal(t, "new-refresh", session.RefreshToken)
			discarded = true
			return nil
		},
	}
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, server.URL+"/xrpc/app.test.endpoint", nil)
	require.NoError(t, err)
	_, err = auth.DoWithAuth(server.Client(), request, syntax.NSID("app.test.endpoint"))
	require.ErrorIs(t, err, ErrCredentialsChanged)
	require.True(t, discarded)
}

func TestRevokeSessionDetachedSurvivesRequestCancellation(t *testing.T) {
	requestCtx, cancel := context.WithCancel(t.Context())
	cancel()
	revoked := false
	revokeSessionDetached(requestCtx, testSessionRevoker(func(ctx context.Context) error {
		require.NoError(t, ctx.Err())
		revoked = true
		return nil
	}))
	require.True(t, revoked)
}

func TestAppPasswordAuthDoesNotReplayNonReplayableBody(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/xrpc/app.test.endpoint":
			requests++
			response.WriteHeader(http.StatusBadRequest)
			_, _ = response.Write([]byte(`{"error":"ExpiredToken"}`))
		case "/xrpc/com.atproto.server.refreshSession":
			_, _ = response.Write([]byte(`{"accessJwt":"` + testAccessToken(t, "com.atproto.appPass") + `","refreshJwt":"new-refresh","did":"did:plc:apppasswordtest"}`))
		}
	}))
	defer server.Close()

	auth := &appPasswordAuth{
		session: testPasswordSession(t, server.URL, "old-access", "old-refresh"),
		recreate: func(context.Context) (atclient.PasswordSessionData, error) {
			return atclient.PasswordSessionData{}, errors.New("unexpected recreation")
		},
		persist: func(context.Context, atclient.PasswordSessionData) error { return nil },
	}
	body := struct{ *strings.Reader }{strings.NewReader("not-replayable")}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/xrpc/app.test.endpoint", body)
	require.NoError(t, err)
	require.Nil(t, request.GetBody)
	_, err = auth.DoWithAuth(server.Client(), request, syntax.NSID("app.test.endpoint"))
	require.ErrorIs(t, err, ErrRequestNotReplayable)
	require.Equal(t, 1, requests)
}

func TestIsAppPasswordRejected(t *testing.T) {
	require.True(t, IsAppPasswordRejected(ErrNotAppPassword))
	require.True(t, IsAppPasswordRejected(ErrPrivilegedAppPassword))
	require.True(t, IsAppPasswordRejected(&atclient.APIError{StatusCode: http.StatusUnauthorized, Name: "AuthenticationRequired"}))
	require.True(t, IsAppPasswordRejected(&atclient.APIError{StatusCode: http.StatusBadRequest, Name: "InvalidLogin"}))
	require.False(t, IsAppPasswordRejected(&atclient.APIError{StatusCode: http.StatusUnauthorized, Name: "AccountTakedown"}))
	require.False(t, IsAppPasswordRejected(&atclient.APIError{StatusCode: http.StatusTooManyRequests, Name: "RateLimitExceeded"}))
	require.False(t, IsAppPasswordRejected(&atclient.APIError{StatusCode: http.StatusServiceUnavailable}))
	require.True(t, IsAppPasswordAccountUnavailable(&atclient.APIError{StatusCode: http.StatusUnauthorized, Name: "AccountTakedown"}))
}

func TestValidateAppPasswordAccessTokenRejectsPrivilegedScope(t *testing.T) {
	require.ErrorIs(t, validateAppPasswordAccessToken(testAccessToken(t, "com.atproto.appPassPrivileged")), ErrPrivilegedAppPassword)
}
