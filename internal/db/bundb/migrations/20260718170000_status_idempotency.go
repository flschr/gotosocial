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
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if err := addColumn(ctx, tx, (*gtsmodel.Status)(nil), "IdempotencyKey"); err != nil {
				return err
			}

			_, err := tx.NewCreateIndex().
				Index("statuses_idempotency_key_idx").
				Table("statuses").
				Column("account_id", "created_with_application_id", "idempotency_key").
				Unique().
				Where("idempotency_key IS NOT NULL").
				Exec(ctx)
			return err
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
