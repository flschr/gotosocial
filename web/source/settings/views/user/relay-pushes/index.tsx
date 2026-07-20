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
import { useRelayPushesQuery } from "../../../lib/query/user/relay-pushes";
import Loading from "../../../components/loading";
import { Error } from "../../../components/error";
import { RelayConnection } from "../../../lib/types/relay";
import { PageableList } from "../../../components/pageable-list";
import RelayListEntry from "../../../components/relaylistentry";

export default function RelayPushesOverview() {
	
	const {
		data,
		isLoading,
		isFetching,
		isSuccess,
		isError,
		error,
	} = useRelayPushesQuery();
	
	if (isLoading || isFetching) {
		return <Loading />;
	}

	if (isError) {
		return <Error error={error} />;
	}

	if (data === undefined) {
		throw "undefined data";
	}

	const itemToEntry = (conn: RelayConnection) => {
		return <RelayListEntry conn={conn} />;
	};
	
	return (
		<div className="relay-connections">
			<div className="form-section-docs">
				<h1>Relay Pushes</h1>
				<p>
					On this page you can see an overview of relay connections
					you've created in order to push your posts to relays.
				</p>
				<a
					href="https://docs.gotosocial.org/en/stable/user_guide/relay_pushes/"
					target="_blank"
					className="docslink"
					rel="noreferrer"
				>
					Learn more about relay pushes (opens in a new tab)
				</a>
			</div>
			<PageableList
				isLoading={isLoading}
				isFetching={isFetching}
				isSuccess={isSuccess}
				isError={isError}
				error={error}
				items={data}
				itemToEntry={itemToEntry}
				emptyMessage="You have not created any relay pushes yet."
			/>
		</div>
	);
}
