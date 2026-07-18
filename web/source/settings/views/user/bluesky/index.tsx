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
import Loading from "../../../components/loading";
import { Error as ErrorC } from "../../../components/error";
import { Checkbox } from "../../../components/form/inputs";
import MutationButton from "../../../components/form/mutation-button";
import { useBoolInput } from "../../../lib/form";
import useFormSubmit from "../../../lib/form/submit";
import type { BlueskyConnection } from "../../../lib/types/bluesky";
import {
	useBlueskyConnectionQuery,
	useUpdateBlueskySettingsMutation,
} from "../../../lib/query/user/bluesky";

export default function BlueskySettings() {
	const query = useBlueskyConnectionQuery();

	if (query.isLoading || query.isFetching) {
		return <Loading />;
	}
	if (query.isError) {
		return <ErrorC error={query.error} />;
	}
	if (!query.data) {
		return <ErrorC error={new Error("Bluesky connection was undefined")} />;
	}

	return <BlueskySettingsForm connection={query.data} />;
}

function BlueskySettingsForm({ connection }: { connection: BlueskyConnection }) {
	const form = {
		crosspostPublic: useBoolInput("crosspost_public", { source: connection }),
		showProfileFollow: useBoolInput("show_profile_follow", { source: connection }),
	};
	const [submitForm, result] = useFormSubmit(form, useUpdateBlueskySettingsMutation());

	return (
		<form className="bluesky-settings" onSubmit={submitForm}>
			<div className="form-section-docs">
				<h1>Bluesky</h1>
				<p>Connect your existing Bluesky account to publish selected posts and handle Bluesky replies from Mastodon clients.</p>
			</div>
			{connection.connected ? <>
				<div className="info">
					Connected as <a href={connection.profile_url} target="_blank" rel="noreferrer">@{connection.handle}</a>
				</div>
				<Checkbox field={form.crosspostPublic} label="Automatically publish public posts to Bluesky" />
				<small>Replies, mentions, boosts, polls, imports, and non-public posts stay on GoToSocial.</small>
				<Checkbox field={form.showProfileFollow} label="Show a ‘Follow on Bluesky’ button on my public profile" />
				<MutationButton disabled={false} label="Save settings" result={result} />
			</> : <>
				<div className="info">No Bluesky account is connected yet.</div>
				<button type="button" disabled>Connect Bluesky account</button>
				<small>The secure Bluesky authorization flow is the next implementation step.</small>
			</>}
		</form>
	);
}
