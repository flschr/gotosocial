// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/uptrace/bun"
)

func init() {
	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(migrateBlueskyQuoteBoostCleanup, down); err != nil {
		panic(err)
	}
}

func migrateBlueskyQuoteBoostCleanup(ctx context.Context, db *bun.DB) error {
	_, err := db.NewAddColumn().
		Model((*gtsmodel.BlueskyConnection)(nil)).
		ColumnExpr("? TIMESTAMPTZ", bun.Ident("quote_boost_cleaned_at")).
		Exec(ctx)
	return err
}
