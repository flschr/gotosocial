// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

// ApplyInstanceSettingsToConfig updates runtime configuration from the
// database-backed GoToSocial Plus instance settings.
func ApplyInstanceSettingsToConfig(settings *gtsmodel.InstanceSettings) {
	config.SetAccountsUseAccountDomainInAcct(settings.AccountsUseAccountDomainInAcct)
	config.SetAccountsHideLocalRoles(settings.AccountsHideLocalRoles)
	config.SetStatusesPreviewCards(settings.StatusesPreviewCards)
	config.SetStatusesHideQuoteFallback(settings.StatusesHideQuoteFallback)
}
