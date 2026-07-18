// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package bundb

import (
	"context"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/uptrace/bun"
)

type blueskyDB struct {
	db *bun.DB
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

func (b *blueskyDB) DeleteBlueskyInteraction(ctx context.Context, id string) error {
	_, err := b.db.NewDelete().Model((*gtsmodel.BlueskyInteraction)(nil)).Where("? = ?", bun.Ident("id"), id).Exec(ctx)
	return err
}
