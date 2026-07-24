// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"strings"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

// BlueskyInteractionAccount presents a private Bluesky reply author through
// the Mastodon client API. It is not a stored account or ActivityPub actor.
func BlueskyInteractionAccount(interaction *gtsmodel.BlueskyInteraction, fallback *apimodel.Account) *apimodel.Account {
	displayName := strings.TrimSpace(interaction.AuthorDisplayName)
	if displayName == "" {
		displayName = interaction.AuthorHandle
	}
	emojiURL := config.GetProtocol() + "://" + config.GetHost() + "/assets/bluesky.png"
	account := *fallback
	account.ID = interaction.AuthorAccountID
	account.Username = interaction.AuthorHandle
	account.Acct = interaction.AuthorHandle
	account.DisplayName = displayName
	account.URL = "https://bsky.app/profile/" + interaction.AuthorDID
	account.Locked = true
	account.Discoverable = false
	account.Indexable = false
	account.NoIndex = true
	account.AvatarMediaID = ""
	account.AvatarDescription = ""
	if interaction.AuthorAvatarURL != "" {
		account.Avatar = interaction.AuthorAvatarURL
		account.AvatarStatic = interaction.AuthorAvatarStaticURL
		if account.AvatarStatic == "" {
			account.AvatarStatic = interaction.AuthorAvatarURL
		}
	}
	account.HeaderMediaID = ""
	account.FollowersCount = 0
	account.FollowingCount = 0
	account.StatusesCount = 0
	account.LastStatusAt = nil
	account.Roles = make([]apimodel.AccountDisplayRole, 0)
	// Apply the instance policy to author-supplied display name content before
	// adding our own service-origin marker. The Bluesky icon is UI metadata,
	// not a profile decoration.
	account.Emojis = nil
	ApplyAccountNameEmojiPolicy(&account)
	account.DisplayName += " :bluesky:"
	account.Emojis = []apimodel.Emoji{{
		Shortcode: "bluesky", URL: emojiURL, StaticURL: emojiURL, VisibleInPicker: false,
	}}
	return &account
}
