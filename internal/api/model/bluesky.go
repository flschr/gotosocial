// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

type BlueskyConnection struct {
	Connected         bool   `json:"connected"`
	Handle            string `json:"handle,omitempty"`
	ProfileURL        string `json:"profile_url,omitempty"`
	CrosspostPublic   bool   `json:"crosspost_public"`
	ShowProfileFollow bool   `json:"show_profile_follow"`
}

type BlueskySettingsUpdateRequest struct {
	CrosspostPublic   *bool `form:"crosspost_public" json:"crosspost_public"`
	ShowProfileFollow *bool `form:"show_profile_follow" json:"show_profile_follow"`
}
