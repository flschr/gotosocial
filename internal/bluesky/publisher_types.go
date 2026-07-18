// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bluesky

type facet struct {
	Index    facetIndex     `json:"index"`
	Features []facetFeature `json:"features"`
}

type facetIndex struct {
	ByteStart int `json:"byteStart"`
	ByteEnd   int `json:"byteEnd"`
}

type facetFeature struct {
	Type string `json:"$type"`
	URI  string `json:"uri,omitempty"`
	Tag  string `json:"tag,omitempty"`
}

type createRecordResponse struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type uploadBlobResponse struct {
	Blob any `json:"blob"`
}
