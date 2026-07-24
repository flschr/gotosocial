// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils

import (
	"regexp"
	"strings"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/regexes"
)

var accountNameWhitespace = regexp.MustCompile(`\s+`)

// ApplyAccountNameEmojiPolicy removes emojis from an API account display name
// when the instance-wide presentation option is enabled. Stored and federated
// account data is never modified.
func ApplyAccountNameEmojiPolicy(account *apimodel.Account) {
	if account == nil || !config.GetAccountsHideNameEmojis() {
		return
	}

	displayName := regexes.UnicodeEmoji.ReplaceAllString(account.DisplayName, "")
	for _, emoji := range account.Emojis {
		displayName = strings.ReplaceAll(displayName, ":"+emoji.Shortcode+":", "")
	}
	account.DisplayName = strings.TrimSpace(accountNameWhitespace.ReplaceAllString(displayName, " "))
}
