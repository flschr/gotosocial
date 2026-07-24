// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"github.com/stretchr/testify/require"
)

func TestApplyAccountNameEmojiPolicy(t *testing.T) {
	oldHideNameEmojis := config.GetAccountsHideNameEmojis()
	config.SetAccountsHideNameEmojis(true)
	t.Cleanup(func() { config.SetAccountsHideNameEmojis(oldHideNameEmojis) })

	account := &apimodel.Account{
		DisplayName: "  Jan 😂 :instance: 👨‍👩‍👧‍👦  Wildboer 🟠 ",
		Emojis: []apimodel.Emoji{{
			Shortcode: "instance",
		}},
	}
	ApplyAccountNameEmojiPolicy(account)
	require.Equal(t, "Jan Wildboer", account.DisplayName)
	require.Len(t, account.Emojis, 1, "emoji metadata remains available for bios and fields")
}

func TestApplyAccountNameEmojiPolicyDisabled(t *testing.T) {
	oldHideNameEmojis := config.GetAccountsHideNameEmojis()
	config.SetAccountsHideNameEmojis(false)
	t.Cleanup(func() { config.SetAccountsHideNameEmojis(oldHideNameEmojis) })
	account := &apimodel.Account{DisplayName: "Jan 😂 :instance:"}
	ApplyAccountNameEmojiPolicy(account)
	require.Equal(t, "Jan 😂 :instance:", account.DisplayName)
}
