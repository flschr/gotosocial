// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"

	dbpkg "code.superseriousbusiness.org/gotosocial/internal/db"
	plusbluesky "code.superseriousbusiness.org/gotosocial/internal/db/bundb/migrations/20260718150000_plus_bluesky"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			for _, model := range []any{
				(*plusbluesky.BlueskyConnection)(nil),
				(*plusbluesky.BlueskyOAuthState)(nil),
				(*plusbluesky.BlueskyDelivery)(nil),
				(*plusbluesky.BlueskyPost)(nil),
				(*plusbluesky.BlueskyInteraction)(nil),
			} {
				if _, err := tx.NewCreateTable().Model(model).Exec(ctx); err != nil {
					return err
				}
			}

			indexes := []struct {
				name   string
				table  string
				column string
			}{
				{"bluesky_oauth_states_account_id_idx", "bluesky_oauth_states", "account_id"},
				{"bluesky_deliveries_next_attempt_at_idx", "bluesky_deliveries", "next_attempt_at"},
				{"bluesky_posts_connection_id_idx", "bluesky_posts", "connection_id"},
				{"bluesky_posts_account_id_idx", "bluesky_posts", "account_id"},
				{"bluesky_interactions_account_id_idx", "bluesky_interactions", "account_id"},
			}
			for _, index := range indexes {
				if err := createIndex(ctx, tx,
					index.name,
					index.table,
					dbpkg.BunExpr{"?", dbpkg.Idents(index.column)},
				); err != nil {
					return err
				}
			}

			return nil
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
