// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bundb

import (
	"context"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/uptrace/bun"
)

type blueskyDB struct {
	db    *bun.DB
	state *state.State
}

func getBlueskyModel[T any](ctx context.Context, db *bun.DB, column string, value any) (*T, error) {
	model := new(T)
	if err := db.NewSelect().Model(model).Where("? = ?", bun.Ident(column), value).Scan(ctx); err != nil {
		return nil, err
	}
	return model, nil
}

func (b *blueskyDB) GetBlueskyConnectionByAccountID(ctx context.Context, accountID string) (*gtsmodel.BlueskyConnection, error) {
	return getBlueskyModel[gtsmodel.BlueskyConnection](ctx, b.db, "account_id", accountID)
}

func (b *blueskyDB) GetBlueskyConnections(ctx context.Context) ([]*gtsmodel.BlueskyConnection, error) {
	connections := make([]*gtsmodel.BlueskyConnection, 0)
	err := b.db.NewSelect().Model(&connections).Scan(ctx)
	return connections, err
}

func (b *blueskyDB) PutBlueskyConnection(ctx context.Context, connection *gtsmodel.BlueskyConnection) error {
	_, err := b.db.NewInsert().Model(connection).Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateBlueskyConnection(ctx context.Context, connection *gtsmodel.BlueskyConnection, columns ...string) error {
	_, err := b.db.NewUpdate().Model(connection).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyConnection(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyConnection)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyDataByAccountID(ctx context.Context, accountID string) error {
	return b.deleteBlueskyModels(ctx, accountID, []any{
		(*gtsmodel.BlueskyDelivery)(nil), (*gtsmodel.BlueskyNotification)(nil), (*gtsmodel.BlueskyPost)(nil),
		(*gtsmodel.BlueskyInteraction)(nil), (*gtsmodel.BlueskyOAuthState)(nil), (*gtsmodel.BlueskyConnection)(nil),
	})
}

func (b *blueskyDB) DeleteBlueskyConnectionDataByAccountID(ctx context.Context, accountID string) error {
	return b.deleteBlueskyModels(ctx, accountID, []any{
		(*gtsmodel.BlueskyDelivery)(nil), (*gtsmodel.BlueskyNotification)(nil), (*gtsmodel.BlueskyInteraction)(nil),
		(*gtsmodel.BlueskyOAuthState)(nil), (*gtsmodel.BlueskyConnection)(nil),
	})
}

func (b *blueskyDB) deleteBlueskyModels(ctx context.Context, accountID string, models []any) error {
	return b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, model := range models {
			if _, err := tx.NewDelete().Model(model).Where("account_id = ?", accountID).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (b *blueskyDB) GetBlueskyHealth(ctx context.Context, accountID string) (*gtsmodel.BlueskyHealth, error) {
	health := new(gtsmodel.BlueskyHealth)
	var err error
	if health.PendingDeliveries, err = b.db.NewSelect().Model((*gtsmodel.BlueskyDelivery)(nil)).Where("account_id = ? AND dead_letter = ?", accountID, false).Count(ctx); err != nil {
		return nil, err
	}
	if health.DeadDeliveries, err = b.db.NewSelect().Model((*gtsmodel.BlueskyDelivery)(nil)).Where("account_id = ? AND dead_letter = ?", accountID, true).Count(ctx); err != nil {
		return nil, err
	}
	if health.DeadNotifications, err = b.db.NewSelect().Model((*gtsmodel.BlueskyNotification)(nil)).Where("account_id = ? AND dead_letter = ?", accountID, true).Count(ctx); err != nil {
		return nil, err
	}
	latest := new(gtsmodel.BlueskyDelivery)
	if err := b.db.NewSelect().Model(latest).Where("account_id = ? AND last_error IS NOT NULL", accountID).Order("updated_at DESC").Limit(1).Scan(ctx); err == nil {
		health.LastError, health.LastErrorAt = latest.LastError, latest.UpdatedAt
	}
	latestNotification := new(gtsmodel.BlueskyNotification)
	if err := b.db.NewSelect().Model(latestNotification).Where("account_id = ? AND last_error IS NOT NULL", accountID).Order("updated_at DESC").Limit(1).Scan(ctx); err == nil && latestNotification.UpdatedAt.After(health.LastErrorAt) {
		health.LastError, health.LastErrorAt = latestNotification.LastError, latestNotification.UpdatedAt
	}
	return health, nil
}

func (b *blueskyDB) RetryBlueskyFailures(ctx context.Context, accountID string, now time.Time) error {
	return b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model((*gtsmodel.BlueskyDelivery)(nil)).Set("attempts = 0, next_attempt_at = ?, claimed_until = NULL, last_error = NULL, dead_letter = ?", now, false).Where("account_id = ?", accountID).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*gtsmodel.BlueskyNotification)(nil)).Set("attempts = 0, next_attempt_at = ?, last_error = NULL, dead_letter = ?", now, false).Where("account_id = ?", accountID).Exec(ctx)
		return err
	})
}
