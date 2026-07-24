// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package processing

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/stretchr/testify/require"
)

func TestApplyInstanceSettingsToConfig(t *testing.T) {
	original := &gtsmodel.InstanceSettings{
		AccountsUseAccountDomainInAcct: config.GetAccountsUseAccountDomainInAcct(),
		AccountsHideLocalRoles:         config.GetAccountsHideLocalRoles(),
		StatusesPreviewCards:           config.GetStatusesPreviewCards(),
		StatusesHideQuoteFallback:      config.GetStatusesHideQuoteFallback(),
	}
	t.Cleanup(func() {
		ApplyInstanceSettingsToConfig(original)
	})

	for _, value := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[value], func(t *testing.T) {
			ApplyInstanceSettingsToConfig(&gtsmodel.InstanceSettings{
				AccountsUseAccountDomainInAcct: value,
				AccountsHideLocalRoles:         value,
				StatusesPreviewCards:           value,
				StatusesHideQuoteFallback:      value,
			})

			require.Equal(t, value, config.GetAccountsUseAccountDomainInAcct())
			require.Equal(t, value, config.GetAccountsHideLocalRoles())
			require.Equal(t, value, config.GetStatusesPreviewCards())
			require.Equal(t, value, config.GetStatusesHideQuoteFallback())
		})
	}
}
