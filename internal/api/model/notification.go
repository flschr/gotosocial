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

package model

// Notification represents a notification of an event relevant to the user.
//
// swagger:model notification
type Notification struct {
	// REQUIRED

	// The id of the notification in the database.
	ID string `json:"id"`
	// The type of event that resulted in the notification.
	// 	follow = Someone followed you. `account` will be set.
	// 	follow_request = Someone requested to follow you. `account` will be set.
	// 	mention = Someone mentioned you in their status. `status` will be set. `account` will be set.
	// 	reblog = Someone boosted one of your statuses. `status` will be set. `account` will be set.
	// 	favourite = Someone favourited one of your statuses. `status` will be set. `account` will be set.
	// 	poll = A poll you have voted in or created has ended. `status` will be set. `account` will be set.
	// 	status = Someone you enabled notifications for has posted a status. `status` will be set. `account` will be set.
	// 	admin.sign_up = Someone has signed up for a new account on the instance. `account` will be set.
	Type string `json:"type"`
	// The timestamp of the notification (ISO 8601 Datetime)
	CreatedAt string `json:"created_at"`
	// The account that performed the action that generated the notification.
	Account *Account `json:"account"`

	// OPTIONAL

	// Status that was the object of the notification, e.g. in mentions, reblogs, favourites, or polls.
	Status *Status `json:"status,omitempty"`
}

// GroupedNotificationsResponse is the response for GET /api/v2/notifications
// (grouped notifications, Mastodon 4.3+). GoToSocial doesn't group
// notifications yet, so each notification is returned as its own group.
//
// swagger:model groupedNotificationsResults
type GroupedNotificationsResponse struct {
	// The grouped notifications themselves.
	NotificationGroups []NotificationGroup `json:"notification_groups"`
	// Accounts referenced by the notification groups.
	Accounts []*Account `json:"accounts"`
	// Statuses referenced by the notification groups.
	Statuses []*Status `json:"statuses"`
}

// NotificationGroup represents a group of related notifications (Mastodon 4.3+).
//
// swagger:model notificationGroup
type NotificationGroup struct {
	// Group key identifying this group of notifications.
	GroupKey string `json:"group_key"`
	// Total number of notifications in this group.
	NotificationsCount int `json:"notifications_count"`
	// The type of event that resulted in the notifications.
	Type string `json:"type"`
	// ID of the most recent notification in the group.
	MostRecentNotificationID string `json:"most_recent_notification_id"`
	// ID of the oldest notification from this group in the returned page.
	PageMinID string `json:"page_min_id,omitempty"`
	// ID of the newest notification from this group in the returned page.
	PageMaxID string `json:"page_max_id,omitempty"`
	// Timestamp of the most recent notification in the returned page (ISO 8601).
	LatestPageNotificationAt string `json:"latest_page_notification_at,omitempty"`
	// IDs of some of the accounts that triggered notifications in this group.
	SampleAccountIDs []string `json:"sample_account_ids"`
	// ID of the status the notifications refer to, if applicable.
	StatusID *string `json:"status_id,omitempty"`
}

/*
	The below functions are added onto the apimodel notification so that it satisfies
	the Timelineable interface in internal/timeline.
*/

func (n *Notification) GetID() string {
	return n.ID
}

func (n *Notification) GetAccountID() string {
	return ""
}

func (n *Notification) GetBoostOfID() string {
	return ""
}

func (n *Notification) GetBoostOfAccountID() string {
	return ""
}
