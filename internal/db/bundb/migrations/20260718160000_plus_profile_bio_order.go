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
			return addColumn(ctx, tx, (*gtsmodel.AccountSettings)(nil), "WebBioFirst")
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
