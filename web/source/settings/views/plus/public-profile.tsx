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
import { useVerifyCredentialsQuery } from "../../lib/query/login";
import { useUpdateCredentialsMutation } from "../../lib/query/user";
import { useUpdateInstanceMutation } from "../../lib/query/admin";
import { useInstanceV1Query } from "../../lib/query/gts-api";
import { useHasPermission } from "../../lib/navigation/util";
import type { Account } from "../../lib/types/account";
import type { InstanceV1 } from "../../lib/types/instance";

export default function PublicProfile() {
	const admin = useHasPermission(["admin"]);

	return (
		<div className="plus-public-profile">
			<div className="form-section-docs">
				<h1>Public Profile</h1>
				<p>Choose how Plus presents public profiles to visitors on the web.</p>
			</div>
			<FormWithData dataQuery={useVerifyCredentialsQuery} DataForm={PublicProfileForm} />
			{admin && <FormWithData dataQuery={useInstanceV1Query} DataForm={InstancePublicProfileForm} />}
		</div>
	);
}

function PublicProfileForm({ data: profile }: { data: Account }) {
	const form = {
		webBioFirst: useBoolInput("web_bio_first", {
			source: profile,
			valueSelector: (account: Account) => account.source?.web_bio_first,
		}),
	};
	const [submitForm, result] = useFormSubmit(form, useUpdateCredentialsMutation(), { changedOnly: true });

	return (
		<form onSubmit={submitForm}>
			<fieldset>
				<legend>My profile</legend>
				<Checkbox field={form.webBioFirst} label="Show my bio before profile links." />
			</fieldset>
			<MutationButton disabled={false} label="Save profile settings" result={result} />
		</form>
	);
}

function InstancePublicProfileForm({ data: instance }: { data: InstanceV1 }) {
	const form = {
		profilesAutoLoadOlderPosts: useBoolInput("profiles_auto_load_older_posts", { source: instance }),
		profilesShowPlusInfo: useBoolInput("profiles_show_plus_info", { source: instance }),
		profilesShowRemoteFollow: useBoolInput("profiles_show_remote_follow", { source: instance }),
	};
	const [submitForm, result] = useFormSubmit(form, useUpdateInstanceMutation());

	return (
		<form onSubmit={submitForm}>
			<fieldset>
				<legend>Instance-wide public profile features</legend>
				<p>These administrator settings apply to every public profile on this instance.</p>
				<Checkbox field={form.profilesAutoLoadOlderPosts} label="Automatically load older posts. The Show older link remains available as a fallback." />
				<Checkbox field={form.profilesShowPlusInfo} label="Show the GoToSocial Plus version and source information." />
				<Checkbox field={form.profilesShowRemoteFollow} label="Show a Follow button that sends visitors back to their own Fediverse server." />
			</fieldset>
			<MutationButton disabled={false} label="Save public profile features" result={result} />
		</form>
	);
}
