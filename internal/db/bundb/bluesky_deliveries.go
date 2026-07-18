// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bundb

import (
	"context"
	"time"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/uptrace/bun"
)

func (b *blueskyDB) ClaimDueBlueskyDeliveries(ctx context.Context, before, claimedUntil time.Time, limit int) ([]*gtsmodel.BlueskyDelivery, error) {
	deliveries := make([]*gtsmodel.BlueskyDelivery, 0, limit)
	err := b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := tx.NewSelect().Model(&deliveries).
			Where("? <= ?", bun.Ident("next_attempt_at"), before).
			Where("dead_letter = ?", false).
			Where("? IS NULL OR ? < ?", bun.Ident("claimed_until"), bun.Ident("claimed_until"), before).
			Order("next_attempt_at ASC").Limit(limit).Scan(ctx); err != nil {
			return err
		}
		claimed := deliveries[:0]
		for _, delivery := range deliveries {
			claimID := id.NewULID()
			result, err := tx.NewUpdate().Model((*gtsmodel.BlueskyDelivery)(nil)).
				Set("claimed_until = ?, claim_id = ?", claimedUntil, claimID).
				Where("id = ?", delivery.ID).
				Where("generation = ?", delivery.Generation).
				Where("claimed_until IS NULL OR claimed_until < ?", before).
				Exec(ctx)
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected == 1 {
				delivery.ClaimID = claimID
				delivery.ClaimedUntil = claimedUntil
				claimed = append(claimed, delivery)
			}
		}
		deliveries = claimed
		return nil
	})
	return deliveries, err
}

func (b *blueskyDB) RenewBlueskyDeliveryClaim(ctx context.Context, id, claimID string, claimedUntil time.Time) (bool, error) {
	result, err := b.db.NewUpdate().Model((*gtsmodel.BlueskyDelivery)(nil)).Set("claimed_until = ?", claimedUntil).
		Where("id = ? AND claim_id = ?", id, claimID).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) GetBlueskyDeliveryByStatusID(ctx context.Context, statusID string) (*gtsmodel.BlueskyDelivery, error) {
	return getBlueskyModel[gtsmodel.BlueskyDelivery](ctx, b.db, "status_id", statusID)
}

func (b *blueskyDB) PutBlueskyDelivery(ctx context.Context, delivery *gtsmodel.BlueskyDelivery) error {
	_, err := b.db.NewInsert().Model(delivery).
		On("CONFLICT (status_id) DO UPDATE").
		Set("account_id = EXCLUDED.account_id").
		Set("action = EXCLUDED.action").
		Set("generation = bluesky_delivery.generation + 1").
		Set("updated_at = EXCLUDED.updated_at").
		Set("attempts = 0").
		Set("next_attempt_at = EXCLUDED.next_attempt_at").
		Set("claim_id = NULL").
		Set("claimed_until = NULL").
		Set("last_error = NULL").
		Set("last_error_code = NULL").
		Set("dead_letter = ?", false).
		Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateBlueskyDelivery(ctx context.Context, delivery *gtsmodel.BlueskyDelivery, columns ...string) error {
	_, err := b.db.NewUpdate().Model(delivery).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateClaimedBlueskyDelivery(ctx context.Context, delivery *gtsmodel.BlueskyDelivery, columns ...string) (bool, error) {
	result, err := b.db.NewUpdate().Model(delivery).Column(columns...).
		Where("id = ? AND generation = ? AND claim_id = ?", delivery.ID, delivery.Generation, delivery.ClaimID).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) CompleteBlueskyDelivery(ctx context.Context, id string, generation int64, claimID string) (bool, error) {
	result, err := b.db.NewDelete().Model((*gtsmodel.BlueskyDelivery)(nil)).
		Where("id = ? AND generation = ? AND claim_id = ?", id, generation, claimID).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) DeleteBlueskyDeliveryByStatusID(ctx context.Context, statusID string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyDelivery)(nil)).Where("? = ?", bun.Ident("status_id"), statusID).Exec(ctx)
	return err
}
