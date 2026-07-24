// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

func TestApplyInstanceSettingsToConfig(t *testing.T) {
	originalAccountsUseAccountDomainInAcct := config.GetAccountsUseAccountDomainInAcct()
	originalAccountsHideLocalRoles := config.GetAccountsHideLocalRoles()
	originalStatusesPreviewCards := config.GetStatusesPreviewCards()
	originalStatusesHideQuoteFallback := config.GetStatusesHideQuoteFallback()
	t.Cleanup(func() {
		config.SetAccountsUseAccountDomainInAcct(originalAccountsUseAccountDomainInAcct)
		config.SetAccountsHideLocalRoles(originalAccountsHideLocalRoles)
		config.SetStatusesPreviewCards(originalStatusesPreviewCards)
		config.SetStatusesHideQuoteFallback(originalStatusesHideQuoteFallback)
	})

	applyInstanceSettingsToConfig(&gtsmodel.InstanceSettings{
		AccountsUseAccountDomainInAcct: true,
		AccountsHideLocalRoles:         true,
		StatusesPreviewCards:           true,
		StatusesHideQuoteFallback:      true,
	})

	if !config.GetAccountsUseAccountDomainInAcct() {
		t.Error("expected accounts-use-account-domain-in-acct to be loaded")
	}
	if !config.GetAccountsHideLocalRoles() {
		t.Error("expected accounts-hide-local-roles to be loaded")
	}
	if !config.GetStatusesPreviewCards() {
		t.Error("expected statuses-preview-cards to be loaded")
	}
	if !config.GetStatusesHideQuoteFallback() {
		t.Error("expected statuses-hide-quote-fallback to be loaded")
	}
}
