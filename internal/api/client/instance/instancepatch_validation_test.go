// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package instance

import (
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/util"
	"github.com/stretchr/testify/assert"
)

func TestValidateInstanceUpdateAcceptsPlusSettings(t *testing.T) {
	tests := map[string]*apimodel.InstanceSettingsUpdateRequest{
		"account domain": {
			AccountsUseAccountDomainInAcct: util.Ptr(true),
		},
		"local role privacy": {
			AccountsHideLocalRoles: util.Ptr(true),
		},
		"preview cards": {
			StatusesPreviewCards: util.Ptr(true),
		},
		"automatic profile loading": {
			ProfilesAutoLoadOlderPosts: util.Ptr(true),
		},
		"profile Plus information": {
			ProfilesShowPlusInfo: util.Ptr(false),
		},
		"profile remote follow": {
			ProfilesShowRemoteFollow: util.Ptr(true),
		},
	}

	for name, form := range tests {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, validateInstanceUpdate(form))
		})
	}
}
