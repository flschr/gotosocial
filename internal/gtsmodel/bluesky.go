// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package gtsmodel

import "time"

// BlueskyConnection links one local account to one Bluesky identity.
type BlueskyConnection struct {
	ID                string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt         time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt         time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID         string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	DID               string    `bun:",nullzero,notnull,unique"`
	Handle            string    `bun:",nullzero,notnull"`
	PDSURL            string    `bun:",nullzero,notnull"`
	OAuthSessionID    string    `bun:",nullzero"`
	OAuthData         []byte    `bun:",nullzero"`
	CrosspostPublic   bool      `bun:",notnull,default:false"`
	ShowProfileFollow bool      `bun:",notnull,default:true"`
}

// BlueskyOAuthState stores one short-lived, encrypted authorization request.
type BlueskyOAuthState struct {
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	State         string    `bun:",nullzero,notnull,unique"`
	EncryptedData []byte    `bun:",nullzero,notnull"`
}

// BlueskyPost stores the exact Bluesky counterpart of a local status.
type BlueskyPost struct {
	ID           string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt    time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt    time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	ConnectionID string    `bun:"type:CHAR(26),nullzero,notnull"`
	AccountID    string    `bun:"type:CHAR(26),nullzero,notnull"`
	StatusID     string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	URI          string    `bun:",nullzero,notnull,unique"`
	CID          string    `bun:",nullzero,notnull"`
	URL          string    `bun:",nullzero,notnull"`
}

// BlueskyInteraction links a private local proxy status to a Bluesky
// interaction so replies can be routed without federating the proxy thread.
type BlueskyInteraction struct {
	ID           string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt    time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID    string    `bun:"type:CHAR(26),nullzero,notnull"`
	StatusID     string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	URI          string    `bun:",nullzero,notnull,unique"`
	CID          string    `bun:",nullzero,notnull"`
	RootURI      string    `bun:",nullzero,notnull"`
	RootCID      string    `bun:",nullzero,notnull"`
	ParentURI    string    `bun:",nullzero,notnull"`
	ParentCID    string    `bun:",nullzero,notnull"`
	AuthorDID    string    `bun:",nullzero,notnull"`
	AuthorHandle string    `bun:",nullzero,notnull"`
	URL          string    `bun:",nullzero,notnull"`
}
