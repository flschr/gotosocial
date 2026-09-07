// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import "time"

type BlueskyConnection struct {
	Connected         bool      `json:"connected"`
	AuthMethod        string    `json:"auth_method,omitempty"`
	DID               string    `json:"did,omitempty"`
	Handle            string    `json:"handle,omitempty"`
	ProfileURL        string    `json:"profile_url,omitempty"`
	CrosspostPublic   bool      `json:"crosspost_public"`
	ShowProfileFollow bool      `json:"show_profile_follow"`
	Configured        bool      `json:"configured"`
	Status            string    `json:"status"`
	StatusMessage     string    `json:"status_message,omitempty"`
	NeedsReconnect    bool      `json:"needs_reconnect"`
	PendingDeliveries int       `json:"pending_deliveries"`
	DeadDeliveries    int       `json:"dead_deliveries"`
	DeadNotifications int       `json:"dead_notifications"`
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

type BlueskyAppPasswordRequest struct {
	Identifier  string `form:"identifier" json:"identifier" binding:"required"`
	AppPassword string `form:"app_password" json:"app_password" binding:"required,max=256"`
}

type BlueskyConnectResponse struct {
	AuthorizationURL string `json:"authorization_url"`
}
