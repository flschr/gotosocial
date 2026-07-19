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
import { Redirect, Route, Router, Switch } from "wouter";
import { ErrorBoundary } from "../../lib/navigation/error";
import { BaseUrlContext, useBaseUrl, useHasPermission } from "../../lib/navigation/util";
import BlueskySettings from "../user/bluesky";
import Overview from "./overview";
import PublicProfile from "./public-profile";
import InstanceFeatures from "./instance-features";

export default function PlusRouter() {
	const parentUrl = useBaseUrl();
	const thisBase = "/plus";
	const absBase = parentUrl + thisBase;
	const admin = useHasPermission(["admin"]);

	return (
		<BaseUrlContext.Provider value={absBase}>
			<Router base={thisBase}>
				<ErrorBoundary>
					<Switch>
						<Route path="/overview" component={Overview} />
						<Route path="/bluesky" component={BlueskySettings} />
						<Route path="/public-profile" component={PublicProfile} />
						{admin && <Route path="/instance-features" component={InstanceFeatures} />}
						<Route><Redirect to="/overview" /></Route>
					</Switch>
				</ErrorBoundary>
			</Router>
		</BaseUrlContext.Provider>
	);
}
