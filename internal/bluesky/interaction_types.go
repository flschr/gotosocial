// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

import (
	"encoding/json"
	"time"
)

type notificationPage struct {
	Cursor        string                `json:"cursor"`
	Notifications []blueskyNotification `json:"notifications"`
}

type blueskyNotification struct {
	URI       string          `json:"uri"`
	CID       string          `json:"cid"`
	Reason    string          `json:"reason"`
	IndexedAt time.Time       `json:"indexedAt"`
	Author    blueskyAuthor   `json:"author"`
	Record    json.RawMessage `json:"record"`
}

type blueskyAuthor struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Avatar      string `json:"avatar"`
}

type blueskyPostRecord struct {
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Facets    []struct {
		Index    facetIndex `json:"index"`
		Features []struct {
			Type string `json:"$type"`
			URI  string `json:"uri"`
			DID  string `json:"did"`
			Tag  string `json:"tag"`
		} `json:"features"`
	} `json:"facets"`
	Embed *blueskyEmbed `json:"embed"`
	Reply *struct {
		Root   blueskyStrongRef `json:"root"`
		Parent blueskyStrongRef `json:"parent"`
	} `json:"reply"`
}

type blueskyEmbed struct {
	Type   string `json:"$type"`
	Images []struct {
		Alt   string `json:"alt"`
		Image struct {
			Ref struct {
				Link string `json:"$link"`
			} `json:"ref"`
		} `json:"image"`
	} `json:"images"`
	External *struct {
		URI         string `json:"uri"`
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"external"`
	Video *struct {
		Ref struct {
			Link string `json:"$link"`
		} `json:"ref"`
	} `json:"video"`
	Alt   string        `json:"alt"`
	Media *blueskyEmbed `json:"media"`
}

type blueskyStrongRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}
