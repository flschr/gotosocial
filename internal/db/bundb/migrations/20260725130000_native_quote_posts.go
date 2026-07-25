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
			// Add nullable quote-relationship columns to statuses.
			// All are nullzero, so no data backfill/batching is needed.
			for _, col := range []string{
				"QuoteID",
				"QuoteURI",
				"QuoteAccountID",
				"QuoteApprovalURI",
			} {
				if err := addColumn(ctx, tx, (*gtsmodel.Status)(nil), col); err != nil {
					return err
				}
			}

			// Index for looking up statuses that quote a given status
			// (e.g. GET /statuses/:id/quotes, and quote-count queries).
			_, err := tx.NewCreateIndex().
				Index("statuses_quote_id_idx").
				Table("statuses").
				Column("quote_id").
				Where("quote_id IS NOT NULL").
				Exec(ctx)
			return err
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
