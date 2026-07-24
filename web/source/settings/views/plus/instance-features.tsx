/*
	GoToSocial
	Copyright (C) GoToSocial Authors admin@gotosocial.org
	SPDX-License-Identifier: AGPL-3.0-or-later

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU Affero General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU Affero General Public License for more details.

	You should have received a copy of the GNU Affero General Public License
	along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

import React from "react";
import { Checkbox } from "../../components/form/inputs";
import MutationButton from "../../components/form/mutation-button";
import { useBoolInput } from "../../lib/form";
import FormWithData from "../../lib/form/form-with-data";
import useFormSubmit from "../../lib/form/submit";
import { useUpdateInstanceMutation } from "../../lib/query/admin";
import { useInstanceV1Query } from "../../lib/query/gts-api";
import type { InstanceV1 } from "../../lib/types/instance";

export default function InstanceFeatures() {
	return <FormWithData dataQuery={useInstanceV1Query} DataForm={InstanceFeaturesForm} />;
}

function InstanceFeaturesForm({ data: instance }: { data: InstanceV1 }) {
	const form = {
		accountsUseAccountDomainInAcct: useBoolInput("accounts_use_account_domain_in_acct", { source: instance }),
		accountsHideLocalRoles: useBoolInput("accounts_hide_local_roles", { source: instance }),
		accountsHideNameEmojis: useBoolInput("accounts_hide_name_emojis", { source: instance }),
		statusesPreviewCards: useBoolInput("statuses_preview_cards", { source: instance }),
		statusesHideQuoteFallback: useBoolInput("statuses_hide_quote_fallback", { source: instance }),
	};
	const [submitForm, result] = useFormSubmit(form, useUpdateInstanceMutation());

	return (
		<form onSubmit={submitForm}>
			<div className="form-section-docs">
				<h1>Instance Features</h1>
				<p>Enable or disable optional Plus behavior for this instance.</p>
			</div>
			<fieldset>
				<legend>Identity and privacy</legend>
				<Checkbox field={form.accountsUseAccountDomainInAcct} label="Use the account domain in local handles (split-domain configuration)." />
				<Checkbox field={form.accountsHideLocalRoles} label="Hide administrator and moderator labels on public profiles." />
				<Checkbox field={form.accountsHideNameEmojis} label="Hide Unicode and custom emojis in account display names." />
			</fieldset>
			<fieldset>
				<legend>Posts and media</legend>
				<Checkbox field={form.statusesPreviewCards} label="Enable link preview cards, including preview images and supported video embeds." />
				<Checkbox field={form.statusesHideQuoteFallback} label="Hide Mastodon's redundant RE: quote link when a matching preview card is visible." />
			</fieldset>
			<MutationButton disabled={false} label="Save instance features" result={result} />
		</form>
	);
}
