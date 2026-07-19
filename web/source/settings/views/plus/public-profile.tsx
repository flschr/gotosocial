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
import type { Account } from "../../lib/types/account";

export default function PublicProfile() {
	return <FormWithData dataQuery={useVerifyCredentialsQuery} DataForm={PublicProfileForm} />;
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
			<div className="form-section-docs">
				<h1>Public Profile</h1>
				<p>Choose how Plus presents your profile to visitors on the web.</p>
			</div>
			<Checkbox field={form.webBioFirst} label="Show my bio before profile links." />
			<MutationButton disabled={false} label="Save profile settings" result={result} />
		</form>
	);
}
