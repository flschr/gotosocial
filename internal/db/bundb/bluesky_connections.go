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
	err := b.db.NewSelect().Model(&connections).
		Where("app_password_data IS NOT NULL OR (oauth_session_id IS NOT NULL AND oauth_data IS NOT NULL)").
		Scan(ctx)
	return connections, err
}

func (b *blueskyDB) GetBlueskyStatusesChangedBetween(ctx context.Context, accountID string, since, before time.Time) ([]*gtsmodel.Status, error) {
	statusIDs := make([]string, 0)
	err := b.db.NewSelect().
		TableExpr("? AS ?", bun.Ident("statuses"), bun.Ident("status")).
		Column("status.id").
		Where("status.account_id = ?", accountID).
		Where("((status.created_at >= ? AND status.created_at < ?) OR (status.edited_at >= ? AND status.edited_at < ?))", since, before, since, before).
		Order("status.id ASC").
		Scan(ctx, &statusIDs)
	if err != nil || len(statusIDs) == 0 {
		return nil, err
	}
	return b.state.DB.GetStatusesByIDs(ctx, statusIDs)
}

func (b *blueskyDB) PutBlueskyConnection(ctx context.Context, connection *gtsmodel.BlueskyConnection) error {
	_, err := b.db.NewInsert().Model(connection).Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateBlueskyConnection(ctx context.Context, connection *gtsmodel.BlueskyConnection, columns ...string) error {
	_, err := b.db.NewUpdate().Model(connection).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) ActivateBlueskyAppPassword(ctx context.Context, connection *gtsmodel.BlueskyConnection, expectedOAuthSessionID string, expectedOAuthData, expectedAppPasswordData []byte) (bool, error) {
	query := b.db.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).
		Set("handle = ?", connection.Handle).
		Set("pds_url = ?", connection.PDSURL).
		Set("app_password_data = ?", connection.AppPasswordData).
		Set("oauth_session_id = NULL, oauth_data = NULL, last_sync_error = NULL, last_sync_error_code = NULL").
		Where("account_id = ?", connection.AccountID).
		Where("id = ?", connection.ID).
		Where("did = ?", connection.DID)
	if expectedOAuthSessionID == "" {
		query = query.Where("oauth_session_id IS NULL")
	} else {
		query = query.Where("oauth_session_id = ?", expectedOAuthSessionID)
	}
	if len(expectedOAuthData) == 0 {
		query = query.Where("oauth_data IS NULL")
	} else {
		query = query.Where("oauth_data = ?", expectedOAuthData)
	}
	if len(expectedAppPasswordData) == 0 {
		query = query.Where("app_password_data IS NULL")
	} else {
		query = query.Where("app_password_data = ?", expectedAppPasswordData)
	}
	result, err := query.Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) UpdateBlueskyAppPasswordData(ctx context.Context, accountID string, expected, replacement []byte) (bool, error) {
	result, err := b.db.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).
		Set("app_password_data = ?", replacement).
		Where("account_id = ?", accountID).
		Where("app_password_data = ?", expected).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) UpdateBlueskyOAuthSession(ctx context.Context, accountID, expectedSessionID string, expectedData []byte, sessionID string, data []byte) (bool, error) {
	query := b.db.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).
		Set("oauth_session_id = ?, oauth_data = ?", sessionID, data).
		Where("account_id = ?", accountID).
		Where("app_password_data IS NULL")
	if expectedSessionID == "" {
		query = query.Where("oauth_session_id IS NULL")
	} else {
		query = query.Where("oauth_session_id = ?", expectedSessionID)
	}
	if len(expectedData) == 0 {
		query = query.Where("oauth_data IS NULL")
	} else {
		query = query.Where("oauth_data = ?", expectedData)
	}
	result, err := query.Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) DeleteBlueskyConnection(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyConnection)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteInactiveBlueskyConnection(ctx context.Context, id string) (bool, error) {
	result, err := b.db.NewDelete().Model((*gtsmodel.BlueskyConnection)(nil)).
		Where("id = ?", id).
		Where("app_password_data IS NULL").
		Where("oauth_session_id IS NULL OR oauth_data IS NULL").
		Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) ClaimBlueskyConnection(ctx context.Context, id string, before, claimedUntil time.Time) (bool, error) {
	result, err := b.db.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).Set("sync_claimed_until = ?", claimedUntil).
		Where("id = ?", id).Where("sync_claimed_until IS NULL OR sync_claimed_until < ?", before).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) RenewBlueskyConnectionClaim(ctx context.Context, id string, expected, claimedUntil time.Time) (bool, error) {
	result, err := b.db.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).Set("sync_claimed_until = ?", claimedUntil).
		Where("id = ? AND sync_claimed_until = ?", id, expected).Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (b *blueskyDB) ReleaseBlueskyConnectionClaim(ctx context.Context, id string, expected time.Time) error {
	_, err := b.db.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).Set("sync_claimed_until = NULL").Where("id = ? AND sync_claimed_until = ?", id, expected).Exec(ctx)
	return err
}

func (b *blueskyDB) DeleteBlueskyDataByAccountID(ctx context.Context, accountID string) error {
	return b.deleteBlueskyModels(ctx, accountID, []any{
		(*gtsmodel.BlueskyDelivery)(nil), (*gtsmodel.BlueskyNotification)(nil), (*gtsmodel.BlueskyPost)(nil),
		(*gtsmodel.BlueskyInteraction)(nil), (*gtsmodel.BlueskyOAuthState)(nil), (*gtsmodel.BlueskyConnection)(nil),
	})
}

func (b *blueskyDB) DeleteBlueskyConnectionDataByAccountID(ctx context.Context, accountID string) error {
	return b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model((*gtsmodel.BlueskyConnection)(nil)).
			Set("oauth_session_id = NULL, oauth_data = NULL, app_password_data = NULL, last_sync_error = NULL, last_sync_error_code = NULL, sync_claimed_until = NULL").
			Where("account_id = ?", accountID).Exec(ctx); err != nil {
			return err
		}
		for _, model := range []any{(*gtsmodel.BlueskyNotification)(nil), (*gtsmodel.BlueskyInteraction)(nil), (*gtsmodel.BlueskyOAuthState)(nil)} {
			if _, err := tx.NewDelete().Model(model).Where("account_id = ?", accountID).Exec(ctx); err != nil {
				return err
			}
		}
		// Cancel never-published work, but retain edits/deletes for statuses with
		// exact remote mappings so they can resume after reconnect.
		_, err := tx.NewDelete().Model((*gtsmodel.BlueskyDelivery)(nil)).
			Where("account_id = ?", accountID).
			Where("status_id NOT IN (SELECT status_id FROM bluesky_posts WHERE account_id = ?)", accountID).
			Exec(ctx)
		return err
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
		health.LastError, health.LastErrorCode, health.LastErrorAt = latest.LastError, latest.LastErrorCode, latest.UpdatedAt
	}
	latestNotification := new(gtsmodel.BlueskyNotification)
	if err := b.db.NewSelect().Model(latestNotification).Where("account_id = ? AND last_error IS NOT NULL", accountID).Order("updated_at DESC").Limit(1).Scan(ctx); err == nil && latestNotification.UpdatedAt.After(health.LastErrorAt) {
		health.LastError, health.LastErrorCode, health.LastErrorAt = latestNotification.LastError, latestNotification.LastErrorCode, latestNotification.UpdatedAt
	}
	return health, nil
}

func (b *blueskyDB) RetryBlueskyFailures(ctx context.Context, accountID string, now time.Time) error {
	return b.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().Model((*gtsmodel.BlueskyDelivery)(nil)).Set("attempts = 0, next_attempt_at = ?, claim_id = NULL, claimed_until = NULL, last_error = NULL, last_error_code = NULL, dead_letter = ?", now, false).Where("account_id = ?", accountID).Exec(ctx); err != nil {
			return err
		}
		_, err := tx.NewUpdate().Model((*gtsmodel.BlueskyNotification)(nil)).Set("attempts = 0, next_attempt_at = ?, last_error = NULL, last_error_code = NULL, dead_letter = ?", now, false).Where("account_id = ?", accountID).Exec(ctx)
		return err
	})
}
