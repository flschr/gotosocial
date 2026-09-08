// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/bluesky"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBlueskyIdentifier(t *testing.T) {
	for input, expected := range map[string]string{
		"fischr.org":     "fischr.org",
		"@fischr.org":    "fischr.org",
		" @fischr.org  ": "fischr.org",
		"did:plc:abc":    "did:plc:abc",
	} {
		actual, err := normalizeBlueskyIdentifier(input)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}

	_, err := normalizeBlueskyIdentifier("@")
	require.Error(t, err)
}

func TestBlueskyConnectionStatusUsesFriendlyMessages(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{OAuthSessionID: "session", OAuthData: []byte("encrypted"), LastSyncError: "resume Bluesky OAuth session: invalid_grant"}
	health := &gtsmodel.BlueskyHealth{LastError: connection.LastSyncError, LastErrorCode: bluesky.ErrorCodeAuth, DeadDeliveries: 1}
	status, message, reconnect := blueskyConnectionStatus(connection, health, true)
	require.Equal(t, "action_required", status)
	require.True(t, reconnect)
	require.NotContains(t, message, "invalid_grant")

	status, message, reconnect = blueskyConnectionStatus(connection, health, false)
	require.Equal(t, "action_required", status)
	require.False(t, reconnect)
	require.Contains(t, message, "server administrator")
}

func TestBlueskyConnectionStatusRequestsReplacementForInvalidAppPassword(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{AppPasswordData: []byte("encrypted")}
	health := &gtsmodel.BlueskyHealth{LastErrorCode: bluesky.ErrorCodeAuth}
	status, message, reconnect := blueskyConnectionStatus(connection, health, true)
	require.Equal(t, "action_required", status)
	require.True(t, reconnect)
	require.Contains(t, message, "Replace")
	require.Contains(t, message, "app password")
}

func TestBlueskyConnectionStatusReportsPendingWork(t *testing.T) {
	status, message, reconnect := blueskyConnectionStatus(
		&gtsmodel.BlueskyConnection{OAuthSessionID: "session", OAuthData: []byte("encrypted")},
		&gtsmodel.BlueskyHealth{PendingDeliveries: 2},
		true,
	)
	require.Equal(t, "syncing", status)
	require.Contains(t, message, "waiting")
	require.False(t, reconnect)
}

func TestBlueskyConnectionStatusReportsDisconnectedSavedAccount(t *testing.T) {
	status, message, reconnect := blueskyConnectionStatus(
		&gtsmodel.BlueskyConnection{DID: "did:plc:saved", Handle: "saved.example"},
		&gtsmodel.BlueskyHealth{},
		true,
	)
	require.Equal(t, "disconnected", status)
	require.Contains(t, message, "Reconnect")
	require.True(t, reconnect)
}

func TestSameBlueskyIdentity(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{DID: "did:plc:expected"}
	require.True(t, sameBlueskyIdentity(connection, "did:plc:expected"))
	require.False(t, sameBlueskyIdentity(connection, "did:plc:different"))
}

func TestOAuthRaceWinnerRequiresSameActiveIdentity(t *testing.T) {
	session := oauth.ClientSessionData{AccountDID: syntax.DID("did:plc:expected")}
	require.True(t, oauthRaceWinnerMatches(
		&gtsmodel.BlueskyConnection{DID: "did:plc:expected", OAuthSessionID: "session", OAuthData: []byte("encrypted")},
		session,
	))
	require.False(t, oauthRaceWinnerMatches(
		&gtsmodel.BlueskyConnection{DID: "did:plc:different", OAuthSessionID: "session", OAuthData: []byte("encrypted")},
		session,
	))
	require.False(t, oauthRaceWinnerMatches(
		&gtsmodel.BlueskyConnection{DID: "did:plc:expected"},
		session,
	))
}
