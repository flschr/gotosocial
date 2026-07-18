// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"fmt"
	"reflect"

	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			const column = "profiles_show_remote_follow"
			exists, err := doesColumnExist(ctx, tx, "instance_settings", column)
			if err != nil {
				return err
			}
			if exists {
				return nil
			}

			var settings *gtsmodel.InstanceSettings
			definition, err := getBunColumnDef(tx, reflect.TypeOf(settings), "ProfilesShowRemoteFollow")
			if err != nil {
				return fmt.Errorf("error making column definition: %w", err)
			}
			if _, err := tx.NewAddColumn().Model(settings).ColumnExpr(definition).Exec(ctx); err != nil {
				return fmt.Errorf("error adding instance_settings.%s: %w", column, err)
			}
			return nil
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
