// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package gtsmodel

import (
	"time"

	"github.com/uptrace/bun"
)

// BlueskyConnection links one local account to one Bluesky identity.
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
	LastSyncErrorCode   string    `bun:",nullzero"`
	SyncClaimedUntil    time.Time `bun:"type:timestamptz,nullzero"`
	OutboxCheckedAt     time.Time `bun:"type:timestamptz,nullzero"`
	CrosspostEnabledAt  time.Time `bun:"type:timestamptz,nullzero"`
	CrosspostPublic     bool      `bun:",notnull,default:false"`
	ShowProfileFollow   bool      `bun:",notnull,default:true"`
}

func (b *BlueskyConnection) Active() bool {
	return b != nil && b.OAuthSessionID != "" && len(b.OAuthData) != 0
}

type BlueskyHealth struct {
	PendingDeliveries int
	DeadDeliveries    int
	DeadNotifications int
	LastError         string
	LastErrorCode     string
	LastErrorAt       time.Time
}

// BlueskyDelivery is a durable, retryable outgoing crosspost job.
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
	LastErrorCode string    `bun:",nullzero"`
	DeadLetter    bool      `bun:",notnull,default:false"`
}

// BlueskyOAuthState stores one short-lived, encrypted authorization request.
type BlueskyOAuthState struct {
	bun.BaseModel `bun:"table:bluesky_oauth_states"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	State         string    `bun:",nullzero,notnull,unique"`
	EncryptedData []byte    `bun:",nullzero,notnull"`
}

// BlueskyNotification is a durable inbox item. Persisting the complete payload
// before advancing the remote watermark prevents pagination volume or one bad
// record from dropping later notifications.
type BlueskyNotification struct {
	bun.BaseModel `bun:"table:bluesky_notifications"`
	ID            string    `bun:"type:CHAR(26),pk,nullzero,notnull,unique"`
	CreatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	UpdatedAt     time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	AccountID     string    `bun:"type:CHAR(26),nullzero,notnull"`
	URI           string    `bun:",nullzero,notnull"`
	Payload       []byte    `bun:",nullzero,notnull"`
	Attempts      int       `bun:",notnull,default:0"`
	NextAttemptAt time.Time `bun:"type:timestamptz,nullzero,notnull,default:current_timestamp"`
	LastError     string    `bun:",nullzero"`
	LastErrorCode string    `bun:",nullzero"`
	DeadLetter    bool      `bun:",notnull,default:false"`
}

// BlueskyPost stores the exact Bluesky counterpart of a local status.
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

// BlueskyInteraction links a private local proxy status to a Bluesky
// interaction so replies can be routed without federating the proxy thread.
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
