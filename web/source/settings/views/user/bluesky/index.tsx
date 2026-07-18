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
import useBlueskyController from "./use-bluesky-controller";

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
	const controller = useBlueskyController(connection);
	const {
		identifier, setIdentifier, confirmDisconnect, setConfirmDisconnect,
		confirmForget, setConfirmForget, disconnect, disconnectResult,
		forget, forgetResult, retry, retryResult, connectResult,
		startConnection, callbackErrorMessage,
	} = controller;
	const form = {
		crosspostPublic: useBoolInput("crosspost_public", { source: connection }),
		showProfileFollow: useBoolInput("show_profile_follow", { source: connection }),
	};
	const [submitForm, result] = useFormSubmit(form, useUpdateBlueskySettingsMutation());

	return (
		<form className="bluesky-settings" onSubmit={submitForm}>
			<div className="form-section-docs">
				<h1>Bluesky</h1>
				<p>Connect your existing Bluesky account to publish eligible public posts and handle Bluesky replies from Mastodon clients.</p>
			</div>
			{connectResult.isError && <ErrorC error={connectResult.error} />}
			{disconnectResult.isError && <ErrorC error={disconnectResult.error} />}
			{forgetResult.isError && <ErrorC error={forgetResult.error} />}
			{retryResult.isError && <ErrorC error={retryResult.error} />}
			{callbackErrorMessage && <ErrorC error={new Error(callbackErrorMessage)} />}
			{connection.connected ? <>
				<div className="info">
					Connected as <a href={connection.profile_url} target="_blank" rel="noreferrer">@{connection.handle}</a>
				</div>
				<Checkbox field={form.crosspostPublic} label="Automatically publish public posts to Bluesky" />
				<small>Replies, mentions, boosts, polls, and non-public posts are not crossposted.</small>
				<Checkbox field={form.showProfileFollow} label="Show a ‘Follow on Bluesky’ button on my public profile" />
				<div className="info">
					<strong>{connection.status === "healthy" ? "Connected" : connection.status === "syncing" ? "Syncing" : "Bluesky needs attention"}</strong>
					{connection.status_message && <p>{connection.status_message}</p>}
					{connection.pending_deliveries > 0 && <small>{connection.pending_deliveries} outgoing post(s) pending.</small>}
					{connection.last_sync_at && <small> Last checked {new Date(connection.last_sync_at).toLocaleString()}.</small>}
				</div>
				{(connection.pending_deliveries > 0 || connection.dead_deliveries > 0 || connection.dead_notifications > 0) &&
					<button type="button" disabled={retryResult.isLoading} onClick={() => void retry()}>Retry Bluesky sync</button>}
				<MutationButton disabled={false} label="Save settings" result={result} />
				{confirmDisconnect ? <div className="info">
					<p>Disconnect Bluesky? Crossposting and reply import will stop, and stored Bluesky credentials will be removed.</p>
					<button type="button" className="button danger" disabled={disconnectResult.isLoading} onClick={() => void disconnect()}>Yes, disconnect Bluesky</button>
					<button type="button" disabled={disconnectResult.isLoading} onClick={() => setConfirmDisconnect(false)}>Cancel</button>
				</div> :
					<button type="button" className="button danger" disabled={disconnectResult.isLoading} onClick={() => setConfirmDisconnect(true)}>Disconnect Bluesky account</button>}
			</> : <>
				<div className="info">{connection.handle
					? <>Bluesky is disconnected. The saved account is <a href={connection.profile_url} target="_blank" rel="noreferrer">@{connection.handle}</a>.</>
					: <>No Bluesky account is connected yet.</>}</div>
				{!connection.handle && <label>
					Bluesky handle
					<input value={identifier} placeholder="your-handle.bsky.social" onChange={(event) => setIdentifier(event.target.value)} />
				</label>}
				<button type="button" disabled={!connection.configured || (!identifier && !connection.handle) || connectResult.isLoading} onClick={() => void startConnection()}>{connection.handle ? "Reconnect saved Bluesky account" : "Connect Bluesky account"}</button>
				{!connection.configured && <small>Bluesky connections are not configured by this server administrator.</small>}
				<small>You will be redirected to your Bluesky provider to approve access. Your password is never shared with GoToSocial.</small>
				{connection.handle && (confirmForget ? <div className="info">
					<p>Forget this saved account and all post mappings? Existing posts will stay on Bluesky, but GoToSocial will no longer be able to edit or delete them.</p>
					<button type="button" className="button danger" disabled={forgetResult.isLoading} onClick={() => void forget()}>Yes, forget saved account</button>
					<button type="button" disabled={forgetResult.isLoading} onClick={() => setConfirmForget(false)}>Cancel</button>
				</div> : <button type="button" className="button danger" disabled={forgetResult.isLoading} onClick={() => setConfirmForget(true)}>Forget saved Bluesky account</button>)}
			</>}
		</form>
	);
}
