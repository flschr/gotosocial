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
import { BaseUrlContext, useBaseUrl } from "../../lib/navigation/util";
import { Redirect, Route, Router, Switch } from "wouter";
import { ErrorBoundary } from "../../lib/navigation/error";
import Profile from "./profile/profile";
import PostSettings from "./posts";
import Account from "./account";
import ExportImport from "./export-import";
import InteractionRequests from "./interactions";
import InteractionRequestDetail from "./interactions/detail";
import Tokens from "./tokens";
import Applications from "./applications";
import NewApp from "./applications/new";
import AppDetail from "./applications/detail";
import { AppTokenCallback } from "./applications/callback";
import Migration from "./migration";
import InstanceInfo from "./instance";
import RelayPushesOverview from "./relay-pushes";
import RelayPushNew from "./relay-pushes/new";
import RelayPushDetail from "./relay-pushes/detail";
import TokenDetail from "./tokens/detail";

/**
 * - /settings/user/profile
 * - /settings/user/account
 * - /settings/user/posts
 * - /settings/user/migration
 * - /settings/user/export-import
 * - /settings/user/tokens
 * - /settings/user/interaction_requests
 * - /settings/user/applications
 * - /settings/user/relay-pushes
 * - /settings/user/instance-info
 */
export default function UserRouter() {
	const baseUrl = useBaseUrl();
	const thisBase = "/user";
	const absBase = baseUrl + thisBase;

	return (
		<BaseUrlContext.Provider value={absBase}>
			<Router base={thisBase}>
				<Switch>
					<Route path="/profile" component={Profile} />
					<Route path="/account" component={Account} />
					<Route path="/posts" component={PostSettings} />
					<Route path="/migration" component={Migration} />
					<Route path="/export-import" component={ExportImport} />
					<Route path="/instance-info" component={InstanceInfo} />
				</Switch>
				<InteractionRequestsRouter />
				<TokensRouter />
				<ApplicationsRouter />
				<RelayPushesRouter />
			</Router>
		</BaseUrlContext.Provider>
	);
}

/**
 * - /settings/users/interaction_requests/search
 * - /settings/users/interaction_requests/{reqId}
 */
function InteractionRequestsRouter() {
	const parentUrl = useBaseUrl();
	const thisBase = "/interaction_requests";
	const absBase = parentUrl + thisBase;

	return (
		<BaseUrlContext.Provider value={absBase}>
			<Router base={thisBase}>
				<ErrorBoundary>
					<Switch>
						<Route path="/search" component={InteractionRequests} />
						<Route path="/:reqId" component={InteractionRequestDetail} />
						<Route><Redirect to="/search"/></Route>
					</Switch>
				</ErrorBoundary>
			</Router>
		</BaseUrlContext.Provider>
	);
}

/**
 * - /settings/user/tokens
 * - /settings/user/tokens/{tokenId}
 */
function TokensRouter() {
	const parentUrl = useBaseUrl();
	const thisBase = "/tokens";
	const absBase = parentUrl + thisBase;

	return (
		<BaseUrlContext.Provider value={absBase}>
			<Router base={thisBase}>
				<ErrorBoundary>
					<Switch>
						<Route path="/" component={Tokens} />
						<Route path="/:tokenId" component={TokenDetail} />
						<Route><Redirect to="/"/></Route>
					</Switch>
				</ErrorBoundary>
			</Router>
		</BaseUrlContext.Provider>
	);
}

/**
 * - /settings/user/applications/search
 * - /settings/user/applications/{appID}
 */
function ApplicationsRouter() {
	const parentUrl = useBaseUrl();
	const thisBase = "/applications";
	const absBase = parentUrl + thisBase;

	return (
		<BaseUrlContext.Provider value={absBase}>
			<Router base={thisBase}>
				<ErrorBoundary>
					<Switch>
						<Route path="/search" component={Applications} />
						<Route path="/new" component={NewApp} />
						<Route path="/callback" component={AppTokenCallback} />
						<Route path="/:appId" component={AppDetail} />
						<Route><Redirect to="/search"/></Route>
					</Switch>
				</ErrorBoundary>
			</Router>
		</BaseUrlContext.Provider>
	);
}

/**
 * - /settings/user/relay-pushes/overview
 * - /settings/user/relay-pushes/new
 * - /settings/user/relay-pushes/:relayPushId
 */
function RelayPushesRouter() {
	const parentUrl = useBaseUrl();
	const thisBase = "/relay-pushes";
	const absBase = parentUrl + thisBase;

	return (
		<BaseUrlContext.Provider value={absBase}>
			<Router base={thisBase}>
				<ErrorBoundary>
					<Switch>
						<Route path="/overview" component={RelayPushesOverview} />
						<Route path="/new" component={RelayPushNew} />
						<Route path="/:relayPushId" component={RelayPushDetail} />
						<Route><Redirect to="/overview" /></Route>
					</Switch>
				</ErrorBoundary>
			</Router>
		</BaseUrlContext.Provider>
	);
}
