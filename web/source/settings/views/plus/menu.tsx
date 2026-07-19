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
import { MenuItem } from "../../lib/navigation/menu";

export default function PlusMenu() {
	return (
		<MenuItem name="GoToSocial Plus" itemUrl="plus" defaultChild="overview">
			<MenuItem name="Overview" itemUrl="overview" icon="fa-star" />
			<MenuItem name="Bluesky" itemUrl="bluesky" icon="fa-cloud" />
			<MenuItem name="Public Profile" itemUrl="public-profile" icon="fa-user-circle" />
			<MenuItem
				name="Instance Features"
				itemUrl="instance-features"
				icon="fa-sliders"
				permissions={["admin"]}
			/>
		</MenuItem>
	);
}
