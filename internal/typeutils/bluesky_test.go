// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/stretchr/testify/require"
)

func TestBlueskyInteractionAccountLooksLikeNativeAuthor(t *testing.T) {
	oldProtocol := config.GetProtocol()
	oldHost := config.GetHost()
	t.Cleanup(func() {
		config.SetProtocol(oldProtocol)
		config.SetHost(oldHost)
	})
	config.SetProtocol("http")
	config.SetHost("localhost:8080")
	fallback := &apimodel.Account{
		ID:           "instance-account",
		Username:     "social.example.org",
		Acct:         "social.example.org",
		DisplayName:  "social.example.org",
		Avatar:       "https://social.example.org/default.png",
		AvatarStatic: "https://social.example.org/default.png",
	}
	interaction := &gtsmodel.BlueskyInteraction{
		AuthorDID:         "did:plc:author",
		AuthorAccountID:   "01K00000000000000000000000",
		AuthorHandle:      "finest.day",
		AuthorDisplayName: "Sebastian",
		AuthorAvatarURL:   "https://social.example.org/files/avatar.jpeg",
	}

	account := BlueskyInteractionAccount(interaction, fallback)

	require.Equal(t, "01K00000000000000000000000", account.ID)
	require.Equal(t, "finest.day", account.Username)
	require.Equal(t, "finest.day", account.Acct)
	require.Equal(t, "Sebastian :bluesky:", account.DisplayName)
	require.Equal(t, "https://bsky.app/profile/did:plc:author", account.URL)
	require.Equal(t, "https://social.example.org/files/avatar.jpeg", account.Avatar)
	require.Equal(t, account.Avatar, account.AvatarStatic)
	require.True(t, account.Locked)
	require.False(t, account.Discoverable)
	require.False(t, account.Indexable)
	require.True(t, account.NoIndex)
	require.Equal(t, []apimodel.Emoji{{
		Shortcode:       "bluesky",
		URL:             "http://localhost:8080/assets/bluesky.png",
		StaticURL:       "http://localhost:8080/assets/bluesky.png",
		VisibleInPicker: false,
	}}, account.Emojis)
	require.Equal(t, "instance-account", fallback.ID)
}
