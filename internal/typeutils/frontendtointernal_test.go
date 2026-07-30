// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package typeutils_test

import (
	"testing"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/typeutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIInteractionPolicyMissingCanQuoteUsesVisibilityDefault(t *testing.T) {
	apiPolicy := &apimodel.InteractionPolicy{
		CanFavourite: apimodel.PolicyRules{
			AutomaticApproval: []apimodel.PolicyValue{apimodel.PolicyValuePublic},
		},
		CanReply: apimodel.PolicyRules{
			AutomaticApproval: []apimodel.PolicyValue{apimodel.PolicyValuePublic},
		},
		CanReblog: apimodel.PolicyRules{
			AutomaticApproval: []apimodel.PolicyValue{apimodel.PolicyValuePublic},
		},
	}

	policy, err := typeutils.APIInteractionPolicyToInteractionPolicy(
		apiPolicy,
		apimodel.VisibilityPublic,
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		gtsmodel.PolicyValues{gtsmodel.PolicyValuePublic},
		policy.CanQuote.AutomaticApproval,
	)
	assert.Empty(t, policy.CanQuote.ManualApproval)
}

func TestAPIInteractionPolicyExplicitCanQuoteIsPreserved(t *testing.T) {
	apiPolicy := &apimodel.InteractionPolicy{
		CanFavourite: apimodel.PolicyRules{},
		CanReply:     apimodel.PolicyRules{},
		CanReblog:    apimodel.PolicyRules{},
		CanQuote: &apimodel.PolicyRules{
			ManualApproval: []apimodel.PolicyValue{apimodel.PolicyValueFollowers},
		},
	}

	policy, err := typeutils.APIInteractionPolicyToInteractionPolicy(
		apiPolicy,
		apimodel.VisibilityPublic,
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		gtsmodel.PolicyValues{gtsmodel.PolicyValueAuthor},
		policy.CanQuote.AutomaticApproval,
	)
	assert.Equal(
		t,
		gtsmodel.PolicyValues{gtsmodel.PolicyValueFollowers},
		policy.CanQuote.ManualApproval,
	)
}
