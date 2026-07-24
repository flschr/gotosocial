// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/id"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, column := range []string{
				"AuthorAccountID",
				"AuthorAvatarURL",
				"AuthorAvatarStaticURL",
			} {
				if err := addColumn(ctx, tx, (*gtsmodel.BlueskyInteraction)(nil), column); err != nil {
					return err
				}
			}
			if err := addColumn(ctx, tx, (*gtsmodel.Status)(nil), "BlueskyInteractionID"); err != nil {
				return err
			}

			var interactions []*gtsmodel.BlueskyInteraction
			if err := tx.NewSelect().Model(&interactions).Scan(ctx); err != nil {
				return err
			}
			for _, interaction := range interactions {
				interaction.AuthorAccountID = id.ULIDFromString("bluesky-author", interaction.AuthorDID)
				if _, err := tx.NewUpdate().Model(interaction).
					Column("author_account_id").
					WherePK().
					Exec(ctx); err != nil {
					return err
				}
				if _, err := tx.NewUpdate().Model((*gtsmodel.Status)(nil)).
					Set("bluesky_interaction_id = ?", interaction.ID).
					Where("id = ?", interaction.StatusID).
					Exec(ctx); err != nil {
					return err
				}
			}
			_, err := tx.NewCreateIndex().
				Index("bluesky_interactions_author_owner_idx").
				Table("bluesky_interactions").
				Column("author_account_id", "account_id").
				Exec(ctx)
			return err
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }
	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
