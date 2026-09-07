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

import { useState } from "react";
import type { BlueskyConnection } from "../../../lib/types/bluesky";
import {
	useConnectBlueskyMutation,
	useConnectBlueskyAppPasswordMutation,
	useDisconnectBlueskyMutation,
	useForgetBlueskyMutation,
	useRetryBlueskyMutation,
} from "../../../lib/query/user/bluesky";

export default function useBlueskyController(connection: BlueskyConnection) {
	const [identifier, setIdentifier] = useState("");
	const [appPassword, setAppPassword] = useState("");
	const [confirmDisconnect, setConfirmDisconnect] = useState(false);
	const [confirmForget, setConfirmForget] = useState(false);
	const [connect, connectResult] = useConnectBlueskyMutation();
	const [connectAppPassword, appPasswordResult] = useConnectBlueskyAppPasswordMutation();
	const [disconnect, disconnectResult] = useDisconnectBlueskyMutation();
	const [forget, forgetResult] = useForgetBlueskyMutation();
	const [retry, retryResult] = useRetryBlueskyMutation();
	const callbackError = new URLSearchParams(window.location.search).get("error");
	const callbackErrorMessage = callbackError === "wrong_account"
		? `This GoToSocial account is linked to @${connection.handle}. Sign in to that same Bluesky account, or forget the saved account first.`
		: callbackError ? "Bluesky could not be connected. Please try again." : undefined;

	const startConnection = async () => {
		const normalizedIdentifier = identifier.trim().replace(/^@/, "");
		const response = await connect({ identifier: normalizedIdentifier || connection.handle || "" }).unwrap();
		window.location.assign(response.authorization_url);
	};
	const startAppPasswordConnection = async () => {
		const normalizedIdentifier = identifier.trim().replace(/^@/, "");
		await connectAppPassword({
			identifier: normalizedIdentifier || connection.handle || "",
			app_password: appPassword,
		}).unwrap();
		setAppPassword("");
	};

	return {
		identifier, setIdentifier, appPassword, setAppPassword,
		confirmDisconnect, setConfirmDisconnect,
		confirmForget, setConfirmForget,
		disconnect, disconnectResult,
		forget, forgetResult,
		retry, retryResult,
		connectResult, startConnection, appPasswordResult, startAppPasswordConnection,
		callbackErrorMessage,
	};
}
