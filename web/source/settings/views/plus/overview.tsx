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
import Loading from "../../components/loading";
import { useInstanceV2Query } from "../../lib/query/gts-api";
import { plusFeatures } from "./features";

export default function Overview() {
	const { data: instance, isLoading, isFetching } = useInstanceV2Query();

	if (isLoading || isFetching) {
		return <Loading />;
	}
	if (!instance) {
		throw new Error("could not fetch instance information");
	}

	return (
		<div className="plus-overview">
			<div className="form-section-docs">
				<h1>GoToSocial Plus</h1>
				<p>A focused set of optional improvements built on top of GoToSocial.</p>
			</div>
			<div className="info plus-version">
				<span>Running version</span>
				<strong>{instance.version}</strong>
				<a href={instance.source_url} target="_blank" rel="noreferrer">View source and feature documentation</a>
			</div>
			<section aria-labelledby="plus-features-title">
				<h2 id="plus-features-title">Included features</h2>
				<div className="plus-feature-grid">
					{plusFeatures.map((feature) => (
						<article className="plus-feature-card" key={feature.title}>
							<h3>{feature.title}</h3>
							<p>{feature.description}</p>
						</article>
					))}
				</div>
			</section>
		</div>
	);
}
