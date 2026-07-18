// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package plusbluesky

import (
	"time"

	"github.com/uptrace/bun"
)

type BlueskyConnection struct {
	bun.BaseModel       `bun:"table:bluesky_connections"`
	ID                  string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt           time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt           time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID           string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	DID                 string    `bun:"did,nullzero,notnull,unique"`
	Handle              string    `bun:",nullzero,notnull"`
	PDSURL              string    `bun:"pds_url,nullzero,notnull"`
	OAuthSessionID      string    `bun:"oauth_session_id,nullzero"`
	OAuthData           []byte    `bun:"oauth_data,nullzero"`
	NotificationsSeenAt time.Time `bun:"type:timestamptz,nullzero"`
	LastSyncAt          time.Time `bun:"type:timestamptz,nullzero"`
	LastSyncError       string    `bun:",nullzero"`
	CrosspostPublic     bool      `bun:",notnull,default:false"`
	ShowProfileFollow   bool      `bun:",notnull,default:true"`
}

type BlueskyDelivery struct {
	bun.BaseModel `bun:"table:bluesky_deliveries"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	StatusID      string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	Action        string    `bun:",nullzero,notnull,default:'upsert'"`
	Attempts      int       `bun:",notnull,default:0"`
	NextAttemptAt time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	ClaimedUntil  time.Time `bun:"type:timestamptz,nullzero"`
	LastError     string    `bun:",nullzero"`
	DeadLetter    bool      `bun:",notnull,default:false"`
}

type BlueskyOAuthState struct {
	bun.BaseModel `bun:"table:bluesky_oauth_states"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	State         string    `bun:",nullzero,notnull,unique"`
	EncryptedData []byte    `bun:",nullzero,notnull"`
}

type BlueskyNotification struct {
	bun.BaseModel `bun:"table:bluesky_notifications,unique:bluesky_notifications_account_uri"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull,unique:bluesky_notifications_account_uri"`
	URI           string    `bun:",nullzero,notnull,unique:bluesky_notifications_account_uri"`
	Payload       []byte    `bun:",nullzero,notnull"`
	Attempts      int       `bun:",notnull,default:0"`
	NextAttemptAt time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	LastError     string    `bun:",nullzero"`
	DeadLetter    bool      `bun:",notnull,default:false"`
}

type BlueskyPost struct {
	bun.BaseModel `bun:"table:bluesky_posts"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	ConnectionID  string    `bun:"type:CHAR(26),nullzero,notnull"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	StatusID      string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	URI           string    `bun:",nullzero,notnull,unique"`
	CID           string    `bun:",nullzero,notnull"`
	RootURI       string    `bun:",nullzero"`
	RootCID       string    `bun:",nullzero"`
	ParentURI     string    `bun:",nullzero"`
	ParentCID     string    `bun:",nullzero"`
	URL           string    `bun:",nullzero,notnull"`
}

type BlueskyInteraction struct {
	bun.BaseModel `bun:"table:bluesky_interactions"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	LastCheckedAt time.Time `bun:"type:timestamptz,nullzero"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	StatusID      string    `bun:"type:CHAR(26),nullzero,notnull,unique"`
	URI           string    `bun:",nullzero,notnull,unique"`
	CID           string    `bun:",nullzero,notnull"`
	RootURI       string    `bun:",nullzero,notnull"`
	RootCID       string    `bun:",nullzero,notnull"`
	ParentURI     string    `bun:",nullzero,notnull"`
	ParentCID     string    `bun:",nullzero,notnull"`
	AuthorDID     string    `bun:",nullzero,notnull"`
	AuthorHandle  string    `bun:",nullzero,notnull"`
	URL           string    `bun:",nullzero,notnull"`
}
