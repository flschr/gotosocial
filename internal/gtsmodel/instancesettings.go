// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package gtsmodel

// InstanceSettings represents settings for the instance.
type InstanceSettings struct {
	// ID of this item in the database.
	//
	// Note: no need to set this as "unique", since
	// there will only ever be one entry in this table.
	ID string `bun:"type:CHAR(26),pk,nullzero,notnull"`

	// Title of the instance.
	Title string `bun:""`

	// Short description of the instance
	ShortDescription string `bun:""`

	// Raw text version of short
	// description (before parsing).
	ShortDescriptionText string `bun:""`

	// Longer description of the instance.
	Description string `bun:""`

	// Raw text version of long
	// description (before parsing).
	DescriptionText string `bun:""`

	// Custom CSS for the instance.
	CustomCSS string `bun:",nullzero"`

	// Use the account domain, rather than the host domain, in local acct values.
	AccountsUseAccountDomainInAcct bool `bun:",notnull,default:false"`

	// Hide local administrator and moderator role labels from public responses.
	AccountsHideLocalRoles bool `bun:",notnull,default:false"`

	// Generate and expose link preview cards for statuses.
	StatusesPreviewCards bool `bun:",notnull,default:false"`

	// Automatically load older posts on public profile pages.
	ProfilesAutoLoadOlderPosts bool `bun:",notnull,default:false"`

	// Show GoToSocial Plus information on public profile pages.
	ProfilesShowPlusInfo bool `bun:",notnull,default:true"`

	// Show a remote-follow action on public profile pages.
	ProfilesShowRemoteFollow bool `bun:",notnull,default:false"`

	// Terms and conditions of the instance.
	Terms string `bun:""`

	// Raw text version of terms (before parsing).
	TermsText string `bun:""`

	// Contact email address for the instance
	ContactEmail string `bun:""`

	// Username of the contact account for the instance
	ContactAccountUsername string `bun:",nullzero"`

	// Contact account ID in the database for the instance
	ContactAccountID string `bun:"type:CHAR(26),nullzero"`

	// Account corresponding to contactAccountID.
	// Field not stored in the db.
	ContactAccount *Account `bun:"-"`

	// List of instance rules.
	// Field not stored in the db.
	Rules []Rule `bun:"-"`
}
