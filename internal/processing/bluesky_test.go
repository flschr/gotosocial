// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/stretchr/testify/require"
)

func TestBlueskyConnectionStatusUsesFriendlyMessages(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{LastSyncError: "resume Bluesky OAuth session: invalid_grant"}
	health := &gtsmodel.BlueskyHealth{LastError: connection.LastSyncError, DeadDeliveries: 1}
	status, message, reconnect := blueskyConnectionStatus(connection, health, true)
	require.Equal(t, "action_required", status)
	require.True(t, reconnect)
	require.NotContains(t, message, "invalid_grant")

	status, message, reconnect = blueskyConnectionStatus(connection, health, false)
	require.Equal(t, "action_required", status)
	require.False(t, reconnect)
	require.Contains(t, message, "server administrator")
}

func TestBlueskyConnectionStatusReportsPendingWork(t *testing.T) {
	status, message, reconnect := blueskyConnectionStatus(
		&gtsmodel.BlueskyConnection{},
		&gtsmodel.BlueskyHealth{PendingDeliveries: 2},
		true,
	)
	require.Equal(t, "syncing", status)
	require.Contains(t, message, "waiting")
	require.False(t, reconnect)
}
