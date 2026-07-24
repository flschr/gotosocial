// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/stretchr/testify/require"
)

func TestBlueskyInteractionAccountLooksLikeNativeAuthor(t *testing.T) {
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
		AuthorHandle:      "finest.day",
		AuthorDisplayName: "Sebastian",
		AuthorAvatar:      "https://cdn.bsky.app/avatar.jpeg",
	}

	account := blueskyInteractionAccount(interaction, fallback)

	require.Equal(t, "bluesky:did:plc:author", account.ID)
	require.Equal(t, "finest.day", account.Username)
	require.Equal(t, "finest.day (🦋)", account.Acct)
	require.Equal(t, "Sebastian", account.DisplayName)
	require.Equal(t, "https://bsky.app/profile/did:plc:author", account.URL)
	require.Equal(t, "https://cdn.bsky.app/avatar.jpeg", account.Avatar)
	require.Equal(t, account.Avatar, account.AvatarStatic)
	require.Equal(t, "instance-account", fallback.ID)
}
