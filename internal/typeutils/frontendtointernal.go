// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package typeutils

import (
	"fmt"
	"net/url"
	"slices"

	apimodel "code.superseriousbusiness.org/gotosocial/internal/api/model"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

func APIVisToVis(m apimodel.Visibility) gtsmodel.Visibility {
	switch m {
	case apimodel.VisibilityPublic:
		return gtsmodel.VisibilityPublic
	case apimodel.VisibilityUnlisted:
		return gtsmodel.VisibilityUnlocked
	case apimodel.VisibilityPrivate:
		return gtsmodel.VisibilityFollowersOnly
	case apimodel.VisibilityMutualsOnly:
		return gtsmodel.VisibilityMutualsOnly
	case apimodel.VisibilityDirect:
		return gtsmodel.VisibilityDirect
	case apimodel.VisibilityNone:
		return gtsmodel.VisibilityNone
	}
	return 0
}

func APIContentTypeToContentType(m apimodel.StatusContentType) gtsmodel.StatusContentType {
	switch m {
	case apimodel.StatusContentTypePlain:
		return gtsmodel.StatusContentTypePlain
	case apimodel.StatusContentTypeMarkdown:
		return gtsmodel.StatusContentTypeMarkdown
	}
	return 0
}

func APIMarkerNameToMarkerName(m apimodel.MarkerName) gtsmodel.MarkerName {
	switch m {
	case apimodel.MarkerNameHome:
		return gtsmodel.MarkerNameHome
	case apimodel.MarkerNameNotifications:
		return gtsmodel.MarkerNameNotifications
	}
	return ""
}

func APIFilterActionToFilterAction(m apimodel.FilterAction) gtsmodel.FilterAction {
	switch m {
	case apimodel.FilterActionWarn:
		return gtsmodel.FilterActionWarn
	case apimodel.FilterActionHide:
		return gtsmodel.FilterActionHide
	case apimodel.FilterActionBlur:
		return gtsmodel.FilterActionBlur
	}
	return gtsmodel.FilterActionNone
}

func APIPolicyValueToPolicyValue(u apimodel.PolicyValue) (gtsmodel.PolicyValue, error) {
	switch u {
	case apimodel.PolicyValuePublic:
		return gtsmodel.PolicyValuePublic, nil

	case apimodel.PolicyValueFollowers:
		return gtsmodel.PolicyValueFollowers, nil

	case apimodel.PolicyValueFollowing:
		return gtsmodel.PolicyValueFollowing, nil

	case apimodel.PolicyValueMutuals:
		return gtsmodel.PolicyValueMutuals, nil

	case apimodel.PolicyValueMentioned:
		return gtsmodel.PolicyValueMentioned, nil

	case apimodel.PolicyValueAuthor:
		return gtsmodel.PolicyValueAuthor, nil

	case apimodel.PolicyValueMe:
		err := fmt.Errorf("policyURI %s has no corresponding internal model", apimodel.PolicyValueMe)
		return "", err

	default:
		// Parse URI to ensure it's a
		// url with a valid protocol.
		url, err := url.Parse(string(u))
		if err != nil {
			err := fmt.Errorf("could not parse non-predefined policy value as uri: %w", err)
			return "", err
		}

		if url.Host != "http" && url.Host != "https" {
			err := fmt.Errorf("non-predefined policy values must have protocol 'http' or 'https' (%s)", u)
			return "", err
		}

		return gtsmodel.PolicyValue(u), nil
	}
}

func APIInteractionPolicyToInteractionPolicy(
	p *apimodel.InteractionPolicy,
	v apimodel.Visibility,
) (*gtsmodel.InteractionPolicy, error) {
	visibility := APIVisToVis(v)

	convertURIs := func(apiURIs []apimodel.PolicyValue) (gtsmodel.PolicyValues, error) {
		policyURIs := gtsmodel.PolicyValues{}
		for _, apiURI := range apiURIs {
			uri, err := APIPolicyValueToPolicyValue(apiURI)
			if err != nil {
				return nil, err
			}

			if !uri.FeasibleForVisibility(visibility) {
				err := fmt.Errorf("policyURI %s is not feasible for visibility %s", apiURI, v)
				return nil, err
			}

			policyURIs = append(policyURIs, uri)
		}
		return policyURIs, nil
	}

	canLikeAutomaticApproval, err := convertURIs(p.CanFavourite.AutomaticApproval)
	if err != nil {
		err := fmt.Errorf("error converting %s.can_favourite.automatic_approval: %w", v, err)
		return nil, err
	}

	canLikeManualApproval, err := convertURIs(p.CanFavourite.ManualApproval)
	if err != nil {
		err := fmt.Errorf("error converting %s.can_favourite.manual_approval: %w", v, err)
		return nil, err
	}

	canReplyAutomaticApproval, err := convertURIs(p.CanReply.AutomaticApproval)
	if err != nil {
		err := fmt.Errorf("error converting %s.can_reply.automatic_approval: %w", v, err)
		return nil, err
	}

	canReplyManualApproval, err := convertURIs(p.CanReply.ManualApproval)
	if err != nil {
		err := fmt.Errorf("error converting %s.can_reply.manual_approval: %w", v, err)
		return nil, err
	}

	canAnnounceAutomaticApproval, err := convertURIs(p.CanReblog.AutomaticApproval)
	if err != nil {
		err := fmt.Errorf("error converting %s.can_reblog.automatic_approval: %w", v, err)
		return nil, err
	}

	canAnnounceManualApproval, err := convertURIs(p.CanReblog.ManualApproval)
	if err != nil {
		err := fmt.Errorf("error converting %s.can_reblog.manual_approval: %w", v, err)
		return nil, err
	}

	canQuote := gtsmodel.DefaultCanQuoteFor(visibility)
	if p.CanQuote != nil {
		canQuoteAutomaticApproval, err := convertURIs(p.CanQuote.AutomaticApproval)
		if err != nil {
			err := fmt.Errorf("error converting %s.can_quote.automatic_approval: %w", v, err)
			return nil, err
		}

		canQuoteManualApproval, err := convertURIs(p.CanQuote.ManualApproval)
		if err != nil {
			err := fmt.Errorf("error converting %s.can_quote.manual_approval: %w", v, err)
			return nil, err
		}

		canQuote = &gtsmodel.PolicyRules{
			AutomaticApproval: canQuoteAutomaticApproval,
			ManualApproval:    canQuoteManualApproval,
		}
	}

	// Normalize URIs.
	//
	// 1. Ensure canLikeAlways, canReplyAlways,
	//    and canAnnounceAlways include self
	//    (either explicitly or within public).

	// ensureIncludesSelf adds the "author" PolicyValue
	// to given slice of PolicyValues, if not already
	// explicitly or implicitly included.
	ensureIncludesSelf := func(vals gtsmodel.PolicyValues) gtsmodel.PolicyValues {
		includesSelf := slices.ContainsFunc(
			vals,
			func(uri gtsmodel.PolicyValue) bool {
				return uri == gtsmodel.PolicyValuePublic ||
					uri == gtsmodel.PolicyValueAuthor
			},
		)

		if includesSelf {
			// This slice of policy values
			// already includes self explicitly
			// or implicitly, nothing to change.
			return vals
		}

		// Need to add self/author to
		// this slice of policy values.
		vals = append(vals, gtsmodel.PolicyValueAuthor)
		return vals
	}

	canLikeAutomaticApproval = ensureIncludesSelf(canLikeAutomaticApproval)
	canReplyAutomaticApproval = ensureIncludesSelf(canReplyAutomaticApproval)
	canAnnounceAutomaticApproval = ensureIncludesSelf(canAnnounceAutomaticApproval)
	canQuote.AutomaticApproval = ensureIncludesSelf(canQuote.AutomaticApproval)

	// 2. Ensure canReplyAlways includes mentioned
	//    accounts (either explicitly or within public).
	if !slices.ContainsFunc(
		canReplyAutomaticApproval,
		func(uri gtsmodel.PolicyValue) bool {
			return uri == gtsmodel.PolicyValuePublic ||
				uri == gtsmodel.PolicyValueMentioned
		},
	) {
		canReplyAutomaticApproval = append(
			canReplyAutomaticApproval,
			gtsmodel.PolicyValueMentioned,
		)
	}

	return &gtsmodel.InteractionPolicy{
		CanLike: &gtsmodel.PolicyRules{
			AutomaticApproval: canLikeAutomaticApproval,
			ManualApproval:    canLikeManualApproval,
		},
		CanReply: &gtsmodel.PolicyRules{
			AutomaticApproval: canReplyAutomaticApproval,
			ManualApproval:    canReplyManualApproval,
		},
		CanAnnounce: &gtsmodel.PolicyRules{
			AutomaticApproval: canAnnounceAutomaticApproval,
			ManualApproval:    canAnnounceManualApproval,
		},
		CanQuote: canQuote,
	}, nil
}

func APIWebPushNotificationPolicyToWebPushNotificationPolicy(policy apimodel.WebPushNotificationPolicy) gtsmodel.WebPushNotificationPolicy {
	switch policy {
	case apimodel.WebPushNotificationPolicyAll:
		return gtsmodel.WebPushNotificationPolicyAll
	case apimodel.WebPushNotificationPolicyFollowed:
		return gtsmodel.WebPushNotificationPolicyFollowed
	case apimodel.WebPushNotificationPolicyFollower:
		return gtsmodel.WebPushNotificationPolicyFollower
	case apimodel.WebPushNotificationPolicyNone:
		return gtsmodel.WebPushNotificationPolicyNone
	}
	return 0
}

func APIMediaPolicyToMediaPolicy(policy apimodel.MediaPolicy) gtsmodel.MediaPolicy {
	switch policy {
	case apimodel.MediaPolicyNoAction:
		return gtsmodel.MediaPolicyNoAction
	case apimodel.MediaPolicyMarkSensitive:
		return gtsmodel.MediaPolicyMarkSensitive
	case apimodel.MediaPolicyReject:
		return gtsmodel.MediaPolicyReject
	default:
		return gtsmodel.MediaPolicyUnknown
	}
}

func APIFollowsPolicyToFollowsPolicy(policy apimodel.FollowsPolicy) gtsmodel.FollowsPolicy {
	switch policy {
	case apimodel.FollowsPolicyNoAction:
		return gtsmodel.FollowsPolicyNoAction
	case apimodel.FollowsPolicyManualApproval:
		return gtsmodel.FollowsPolicyManualApproval
	case apimodel.FollowsPolicyRejectNonMutual:
		return gtsmodel.FollowsPolicyRejectNonMutual
	case apimodel.FollowsPolicyRejectAll:
		return gtsmodel.FollowsPolicyRejectAll
	default:
		return gtsmodel.FollowsPolicyUnknown
	}
}

func APIStatusesPolicyToStatusesPolicy(policy apimodel.StatusesPolicy) gtsmodel.StatusesPolicy {
	switch policy {
	case apimodel.StatusesPolicyNoAction:
		return gtsmodel.StatusesPolicyNoAction
	case apimodel.StatusesPolicyFilterWarn:
		return gtsmodel.StatusesPolicyFilterWarn
	case apimodel.StatusesPolicyFilterHide:
		return gtsmodel.StatusesPolicyFilterHide
	default:
		return gtsmodel.StatusesPolicyUnknown
	}
}

func APIAccountsPolicyToAccountsPolicy(policy apimodel.AccountsPolicy) gtsmodel.AccountsPolicy {
	switch policy {
	case apimodel.AccountsPolicyNoAction:
		return gtsmodel.AccountsPolicyNoAction
	case apimodel.AccountsPolicyMute:
		return gtsmodel.AccountsPolicyMute
	default:
		return gtsmodel.AccountsPolicyUnknown
	}
}
