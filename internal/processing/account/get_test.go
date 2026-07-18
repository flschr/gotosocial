// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package account

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/stretchr/testify/require"
)

func TestShowBlueskyProfileRequiresActiveConnection(t *testing.T) {
	connection := &gtsmodel.BlueskyConnection{ShowProfileFollow: true}
	require.False(t, showBlueskyProfile(connection))
	connection.OAuthSessionID = "session"
	connection.OAuthData = []byte("encrypted")
	require.True(t, showBlueskyProfile(connection))
	connection.ShowProfileFollow = false
	require.False(t, showBlueskyProfile(connection))
}
