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
	PutBlueskyConnection(context.Context, *gtsmodel.BlueskyConnection) error
	UpdateBlueskyConnection(context.Context, *gtsmodel.BlueskyConnection, ...string) error
	DeleteBlueskyConnection(context.Context, string) error
	DeleteBlueskyDataByAccountID(context.Context, string) error
	GetBlueskyOAuthState(context.Context, string) (*gtsmodel.BlueskyOAuthState, error)
	PutBlueskyOAuthState(context.Context, *gtsmodel.BlueskyOAuthState) error
	DeleteBlueskyOAuthState(context.Context, string) error
	DeleteExpiredBlueskyOAuthStates(context.Context, time.Time) error

	ClaimDueBlueskyDeliveries(context.Context, time.Time, time.Time, int) ([]*gtsmodel.BlueskyDelivery, error)
	GetBlueskyDeliveryByStatusID(context.Context, string) (*gtsmodel.BlueskyDelivery, error)
	PutBlueskyDelivery(context.Context, *gtsmodel.BlueskyDelivery) error
	UpdateBlueskyDelivery(context.Context, *gtsmodel.BlueskyDelivery, ...string) error
	DeleteBlueskyDeliveryByStatusID(context.Context, string) error

	GetBlueskyPostByStatusID(context.Context, string) (*gtsmodel.BlueskyPost, error)
	GetBlueskyPostByURI(context.Context, string) (*gtsmodel.BlueskyPost, error)
	PutBlueskyPost(context.Context, *gtsmodel.BlueskyPost) error
	UpdateBlueskyPost(context.Context, *gtsmodel.BlueskyPost, ...string) error
	DeleteBlueskyPost(context.Context, string) error

	GetBlueskyInteractionByStatusID(context.Context, string) (*gtsmodel.BlueskyInteraction, error)
	GetBlueskyInteractionByURI(context.Context, string) (*gtsmodel.BlueskyInteraction, error)
	PutBlueskyInteraction(context.Context, *gtsmodel.BlueskyInteraction) error
	DeleteBlueskyInteraction(context.Context, string) error
}
