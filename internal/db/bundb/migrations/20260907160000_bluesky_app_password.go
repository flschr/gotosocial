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
	if err := Migrations.Register(migrateBlueskyAppPassword, down); err != nil {
		panic(err)
	}
}

func migrateBlueskyAppPassword(ctx context.Context, db *bun.DB) error {
	_, err := db.NewAddColumn().
		Model((*gtsmodel.BlueskyConnection)(nil)).
		ColumnExpr("? BYTEA", bun.Ident("app_password_data")).
		Exec(ctx)
	return err
}
