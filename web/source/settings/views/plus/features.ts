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

export interface PlusFeature {
	title: string;
	description: string;
}

// Keep this product-level catalog aligned with GOTOSOCIAL_PLUS.md.
// List stable features, not individual fixes or implementation details.
export const plusFeatures: PlusFeature[] = [
	{
		title: "Native Bluesky integration",
		description: "Connect a Bluesky account, crosspost eligible public posts, and handle Bluesky replies from Mastodon clients."
	},
	{
		title: "Improved public profiles",
		description: "Adds optional remote-follow actions, configurable profile ordering, automatic loading of older posts, and clearer Plus information."
	},
	{
		title: "Better media presentation",
		description: "Shows profile images without forced cropping and provides a centered, accessible media viewer with image descriptions."
	},
	{
		title: "Rich link previews",
		description: "Creates Mastodon-compatible preview cards for links, images, and supported privacy-friendly video embeds."
	},
	{
		title: "Split-domain compatibility",
		description: "Keeps local handles consistent in Mastodon clients when the public account domain differs from the server domain."
	},
	{
		title: "Privacy controls",
		description: "Allows local administrator and moderator labels to stay private on public profiles."
	},
	{
		title: "Reliable publishing and feeds",
		description: "Improves duplicate-safe publishing for compatible clients and keeps public RSS output dependable."
	}
];
