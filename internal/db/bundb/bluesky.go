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
	if err := b.db.NewSelect().Model(&connections).Scan(ctx); err != nil {
		return nil, err
	}
	return connections, nil
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
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyConnection)(nil)).Where("? = ?", bun.Ident("id"), id).Exec(ctx)
	return err
}

// DeleteBlueskyDataByAccountID removes all Bluesky-owned data for an account
// in one transaction. In particular, this guarantees encrypted OAuth material
// cannot survive account deletion.
func (b *blueskyDB) DeleteBlueskyDataByAccountID(ctx context.Context, accountID string) error {
	return b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		models := []any{
			(*gtsmodel.BlueskyDelivery)(nil),
			(*gtsmodel.BlueskyNotification)(nil),
			(*gtsmodel.BlueskyPost)(nil),
			(*gtsmodel.BlueskyInteraction)(nil),
			(*gtsmodel.BlueskyOAuthState)(nil),
			(*gtsmodel.BlueskyConnection)(nil),
		}
		for _, model := range models {
			if _, err := tx.NewDelete().Model(model).Where("? = ?", bun.Ident("account_id"), accountID).Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (b *blueskyDB) PutBlueskyNotification(ctx context.Context, notification *gtsmodel.BlueskyNotification) error {
	_, err := b.db.NewInsert().Model(notification).On("CONFLICT (account_id, uri) DO NOTHING").Exec(ctx)
	return err
}

func (b *blueskyDB) GetDueBlueskyNotifications(ctx context.Context, accountID string, before time.Time, limit int) ([]*gtsmodel.BlueskyNotification, error) {
	notifications := make([]*gtsmodel.BlueskyNotification, 0, limit)
	err := b.db.NewSelect().Model(&notifications).
		Where("account_id = ?", accountID).
		Where("dead_letter = ?", false).
		Where("next_attempt_at <= ?", before).
		Order("created_at ASC").Limit(limit).Scan(ctx)
	return notifications, err
}

func (b *blueskyDB) UpdateBlueskyNotification(ctx context.Context, notification *gtsmodel.BlueskyNotification, columns ...string) error {
	_, err := b.db.NewUpdate().Model(notification).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyNotification(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyNotification)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (b *blueskyDB) GetBlueskyOAuthState(ctx context.Context, state string) (*gtsmodel.BlueskyOAuthState, error) {
	return getBlueskyModel[gtsmodel.BlueskyOAuthState](ctx, b.db, "state", state)
}

func (b *blueskyDB) PutBlueskyOAuthState(ctx context.Context, state *gtsmodel.BlueskyOAuthState) error {
	_, err := b.db.NewInsert().Model(state).Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyOAuthState(ctx context.Context, state string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyOAuthState)(nil)).Where("? = ?", bun.Ident("state"), state).Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteExpiredBlueskyOAuthStates(ctx context.Context, before time.Time) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyOAuthState)(nil)).Where("? < ?", bun.Ident("created_at"), before).Exec(ctx)
	return err
}

func (b *blueskyDB) ClaimDueBlueskyDeliveries(ctx context.Context, before, claimedUntil time.Time, limit int) ([]*gtsmodel.BlueskyDelivery, error) {
	deliveries := make([]*gtsmodel.BlueskyDelivery, 0, limit)
	err := b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.NewSelect().Model(&deliveries).
			Where("? <= ?", bun.Ident("next_attempt_at"), before).
			Where("? IS NULL OR ? < ?", bun.Ident("claimed_until"), bun.Ident("claimed_until"), before).
			Order("next_attempt_at ASC").Limit(limit).Scan(ctx); err != nil {
			return err
		}
		claimed := deliveries[:0]
		for _, delivery := range deliveries {
			result, err := tx.NewUpdate().Model((*gtsmodel.BlueskyDelivery)(nil)).
				Set("claimed_until = ?", claimedUntil).
				Where("id = ?", delivery.ID).
				Where("claimed_until IS NULL OR claimed_until < ?", before).
				Exec(ctx)
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected == 1 {
				delivery.ClaimedUntil = claimedUntil
				claimed = append(claimed, delivery)
			}
		}
		deliveries = claimed
		return nil
	})
	return deliveries, err
}

func (b *blueskyDB) GetBlueskyDeliveryByStatusID(ctx context.Context, statusID string) (*gtsmodel.BlueskyDelivery, error) {
	return getBlueskyModel[gtsmodel.BlueskyDelivery](ctx, b.db, "status_id", statusID)
}

func (b *blueskyDB) PutBlueskyDelivery(ctx context.Context, delivery *gtsmodel.BlueskyDelivery) error {
	_, err := b.db.NewInsert().Model(delivery).Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateBlueskyDelivery(ctx context.Context, delivery *gtsmodel.BlueskyDelivery, columns ...string) error {
	_, err := b.db.NewUpdate().Model(delivery).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyDeliveryByStatusID(ctx context.Context, statusID string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyDelivery)(nil)).Where("? = ?", bun.Ident("status_id"), statusID).Exec(ctx)
	return err
}

func (b *blueskyDB) GetBlueskyPostByStatusID(ctx context.Context, statusID string) (*gtsmodel.BlueskyPost, error) {
	return getBlueskyModel[gtsmodel.BlueskyPost](ctx, b.db, "status_id", statusID)
}

func (b *blueskyDB) GetBlueskyPostByURI(ctx context.Context, uri string) (*gtsmodel.BlueskyPost, error) {
	return getBlueskyModel[gtsmodel.BlueskyPost](ctx, b.db, "uri", uri)
}

func (b *blueskyDB) PutBlueskyPost(ctx context.Context, post *gtsmodel.BlueskyPost) error {
	_, err := b.db.NewInsert().Model(post).Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateBlueskyPost(ctx context.Context, post *gtsmodel.BlueskyPost, columns ...string) error {
	_, err := b.db.NewUpdate().Model(post).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyPost(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyPost)(nil)).Where("? = ?", bun.Ident("id"), id).Exec(ctx)
	return err
}

func (b *blueskyDB) GetBlueskyInteractionByStatusID(ctx context.Context, statusID string) (*gtsmodel.BlueskyInteraction, error) {
	return getBlueskyModel[gtsmodel.BlueskyInteraction](ctx, b.db, "status_id", statusID)
}

func (b *blueskyDB) GetBlueskyInteractionByURI(ctx context.Context, uri string) (*gtsmodel.BlueskyInteraction, error) {
	return getBlueskyModel[gtsmodel.BlueskyInteraction](ctx, b.db, "uri", uri)
}

func (b *blueskyDB) PutBlueskyInteraction(ctx context.Context, interaction *gtsmodel.BlueskyInteraction) error {
	_, err := b.db.NewInsert().Model(interaction).Exec(ctx)
	return err
}

func (b *blueskyDB) PutBlueskyInteractionStatus(ctx context.Context, status *gtsmodel.Status, mention *gtsmodel.Mention, interaction *gtsmodel.BlueskyInteraction) error {
	err := b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if status.ThreadID == "" && status.InReplyToID != "" {
			if err := tx.NewSelect().Table("statuses").Column("thread_id").Where("id = ?", status.InReplyToID).Scan(ctx, &status.ThreadID); err != nil {
				return err
			}
		}
		if err := insertStatus(ctx, tx, status); err != nil {
			return err
		}
		if _, err := tx.NewInsert().Model(mention).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewInsert().Model(interaction).Exec(ctx)
		return err
	})
	if err == nil {
		b.state.Caches.DB.Status.InvalidateIDs("ID", []string{status.ID})
		b.state.Caches.DB.Mention.InvalidateIDs("ID", []string{mention.ID})
	}
	return err
}

func (b *blueskyDB) DeleteBlueskyInteraction(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyInteraction)(nil)).Where("? = ?", bun.Ident("id"), id).Exec(ctx)
	return err
}
