// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import "time"

type BlueskyConnection struct {
	Connected         bool      `json:"connected"`
	Handle            string    `json:"handle,omitempty"`
	ProfileURL        string    `json:"profile_url,omitempty"`
	CrosspostPublic   bool      `json:"crosspost_public"`
	ShowProfileFollow bool      `json:"show_profile_follow"`
	Configured        bool      `json:"configured"`
	PendingDeliveries int       `json:"pending_deliveries"`
	DeadDeliveries    int       `json:"dead_deliveries"`
	DeadNotifications int       `json:"dead_notifications"`
	LastError         string    `json:"last_error,omitempty"`
	LastErrorAt       time.Time `json:"last_error_at,omitempty"`
	LastSyncAt        time.Time `json:"last_sync_at,omitempty"`
}

type BlueskySettingsUpdateRequest struct {
	CrosspostPublic   *bool `form:"crosspost_public" json:"crosspost_public"`
	ShowProfileFollow *bool `form:"show_profile_follow" json:"show_profile_follow"`
}

type BlueskyConnectRequest struct {
	Identifier string `form:"identifier" json:"identifier" binding:"required"`
}

type BlueskyConnectResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}
