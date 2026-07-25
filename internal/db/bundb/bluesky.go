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

func (b *blueskyDB) GetBlueskyPostByStatusID(ctx context.Context, statusID string) (*gtsmodel.BlueskyPost, error) {
	return getBlueskyModel[gtsmodel.BlueskyPost](ctx, b.db, "status_id", statusID)
}

func (b *blueskyDB) GetBlueskyPostByURI(ctx context.Context, uri string) (*gtsmodel.BlueskyPost, error) {
	return getBlueskyModel[gtsmodel.BlueskyPost](ctx, b.db, "uri", uri)
}

func (b *blueskyDB) GetBlueskyPostsByAccountID(ctx context.Context, accountID string) ([]*gtsmodel.BlueskyPost, error) {
	posts := make([]*gtsmodel.BlueskyPost, 0)
	err := b.db.NewSelect().Model(&posts).Where("account_id = ?", accountID).Scan(ctx)
	return posts, err
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

func (b *blueskyDB) GetBlueskyInteractionByID(ctx context.Context, interactionID string) (*gtsmodel.BlueskyInteraction, error) {
	return getBlueskyModel[gtsmodel.BlueskyInteraction](ctx, b.db, "id", interactionID)
}

func (b *blueskyDB) GetBlueskyInteractionByAuthorAccountID(ctx context.Context, authorAccountID, accountID string) (*gtsmodel.BlueskyInteraction, error) {
	interaction := new(gtsmodel.BlueskyInteraction)
	err := b.db.NewSelect().Model(interaction).
		Where("author_account_id = ?", authorAccountID).
		Where("account_id = ?", accountID).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	return interaction, err
}

func (b *blueskyDB) IsBlueskyInteractionAvatar(ctx context.Context, url, thumbnailURL string) (bool, error) {
	if url == "" && thumbnailURL == "" {
		return false, nil
	}
	return b.db.NewSelect().
		Model((*gtsmodel.BlueskyInteraction)(nil)).
		Where(
			"author_avatar_url IN (?) OR author_avatar_static_url IN (?)",
			bun.In([]string{url, thumbnailURL}),
			bun.In([]string{url, thumbnailURL}),
		).
		Exists(ctx)
}

func (b *blueskyDB) GetBlueskyInteractionByURI(ctx context.Context, uri string) (*gtsmodel.BlueskyInteraction, error) {
	return getBlueskyModel[gtsmodel.BlueskyInteraction](ctx, b.db, "uri", uri)
}

func (b *blueskyDB) GetBlueskyInteractionsForReconcile(ctx context.Context, accountID string, limit int) ([]*gtsmodel.BlueskyInteraction, error) {
	interactions := make([]*gtsmodel.BlueskyInteraction, 0, limit)
	query := b.db.NewSelect().Model(&interactions).Where("account_id = ?", accountID).
		OrderExpr("last_checked_at ASC NULLS FIRST")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Scan(ctx)
	return interactions, err
}

func (b *blueskyDB) PutBlueskyInteraction(ctx context.Context, interaction *gtsmodel.BlueskyInteraction) error {
	prepareBlueskyInteraction(interaction)
	_, err := b.db.NewInsert().Model(interaction).Exec(ctx)
	return err
}

func (b *blueskyDB) UpdateBlueskyInteraction(ctx context.Context, interaction *gtsmodel.BlueskyInteraction, columns ...string) error {
	_, err := b.db.NewUpdate().Model(interaction).Column(columns...).WherePK().Exec(ctx)
	return err
}

func (b *blueskyDB) PutBlueskyInteractionStatus(ctx context.Context, status *gtsmodel.Status, mention *gtsmodel.Mention, interaction *gtsmodel.BlueskyInteraction) error {
	prepareBlueskyInteraction(interaction)
	status.BlueskyInteractionID = interaction.ID
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

func prepareBlueskyInteraction(interaction *gtsmodel.BlueskyInteraction) {
	if interaction.AuthorAccountID == "" && interaction.AuthorDID != "" {
		interaction.AuthorAccountID = id.ULIDFromString("bluesky-author", interaction.AuthorDID)
	}
}

func (b *blueskyDB) DeleteBlueskyInteraction(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyInteraction)(nil)).Where("? = ?", bun.Ident("id"), id).Exec(ctx)
	return err
}
