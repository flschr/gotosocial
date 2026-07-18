// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package db

import (
	"context"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
)

// Bluesky contains persistence operations for optional Bluesky connections.
type Bluesky interface {
	GetBlueskyConnectionByAccountID(context.Context, string) (*gtsmodel.BlueskyConnection, error)
	GetBlueskyConnections(context.Context) ([]*gtsmodel.BlueskyConnection, error)
	GetBlueskyStatusesChangedBetween(context.Context, string, time.Time, time.Time) ([]*gtsmodel.Status, error)
	PutBlueskyConnection(context.Context, *gtsmodel.BlueskyConnection) error
	UpdateBlueskyConnection(context.Context, *gtsmodel.BlueskyConnection, ...string) error
	DeleteBlueskyConnection(context.Context, string) error
	ClaimBlueskyConnection(context.Context, string, time.Time, time.Time) (bool, error)
	RenewBlueskyConnectionClaim(context.Context, string, time.Time, time.Time) (bool, error)
	ReleaseBlueskyConnectionClaim(context.Context, string, time.Time) error
	DeleteBlueskyDataByAccountID(context.Context, string) error
	DeleteBlueskyConnectionDataByAccountID(context.Context, string) error
	GetBlueskyHealth(context.Context, string) (*gtsmodel.BlueskyHealth, error)
	RetryBlueskyFailures(context.Context, string, time.Time) error
	GetBlueskyOAuthState(context.Context, string) (*gtsmodel.BlueskyOAuthState, error)
	PutBlueskyOAuthState(context.Context, *gtsmodel.BlueskyOAuthState) error
	DeleteBlueskyOAuthState(context.Context, string) error
	DeleteExpiredBlueskyOAuthStates(context.Context, time.Time) error
	PutBlueskyNotification(context.Context, *gtsmodel.BlueskyNotification) error
	GetDueBlueskyNotifications(context.Context, string, time.Time, int) ([]*gtsmodel.BlueskyNotification, error)
	UpdateBlueskyNotification(context.Context, *gtsmodel.BlueskyNotification, ...string) error
	DeleteBlueskyNotification(context.Context, string) error

	ClaimDueBlueskyDeliveries(context.Context, time.Time, time.Time, int) ([]*gtsmodel.BlueskyDelivery, error)
	RenewBlueskyDeliveryClaim(context.Context, string, string, time.Time) (bool, error)
	GetBlueskyDeliveryByStatusID(context.Context, string) (*gtsmodel.BlueskyDelivery, error)
	PutBlueskyDelivery(context.Context, *gtsmodel.BlueskyDelivery) error
	UpdateBlueskyDelivery(context.Context, *gtsmodel.BlueskyDelivery, ...string) error
	UpdateClaimedBlueskyDelivery(context.Context, *gtsmodel.BlueskyDelivery, ...string) (bool, error)
	CompleteBlueskyDelivery(context.Context, string, int64, string) (bool, error)
	DeleteBlueskyDeliveryByStatusID(context.Context, string) error

	GetBlueskyPostByStatusID(context.Context, string) (*gtsmodel.BlueskyPost, error)
	GetBlueskyPostByURI(context.Context, string) (*gtsmodel.BlueskyPost, error)
	GetBlueskyPostsByAccountID(context.Context, string) ([]*gtsmodel.BlueskyPost, error)
	PutBlueskyPost(context.Context, *gtsmodel.BlueskyPost) error
	UpdateBlueskyPost(context.Context, *gtsmodel.BlueskyPost, ...string) error
	DeleteBlueskyPost(context.Context, string) error

	GetBlueskyInteractionByStatusID(context.Context, string) (*gtsmodel.BlueskyInteraction, error)
	GetBlueskyInteractionByURI(context.Context, string) (*gtsmodel.BlueskyInteraction, error)
	GetBlueskyInteractionsForReconcile(context.Context, string, int) ([]*gtsmodel.BlueskyInteraction, error)
	PutBlueskyInteraction(context.Context, *gtsmodel.BlueskyInteraction) error
	UpdateBlueskyInteraction(context.Context, *gtsmodel.BlueskyInteraction, ...string) error
	PutBlueskyInteractionStatus(context.Context, *gtsmodel.Status, *gtsmodel.Mention, *gtsmodel.BlueskyInteraction) error
	DeleteBlueskyInteraction(context.Context, string) error
}
