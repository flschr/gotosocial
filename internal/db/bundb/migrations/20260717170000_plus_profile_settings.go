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
			var settings *gtsmodel.InstanceSettings
			settingsType := reflect.TypeOf(settings)

			for _, column := range []struct {
				name  string
				field string
			}{
				{"profiles_auto_load_older_posts", "ProfilesAutoLoadOlderPosts"},
				{"profiles_show_plus_info", "ProfilesShowPlusInfo"},
			} {
				exists, err := doesColumnExist(ctx, tx, "instance_settings", column.name)
				if err != nil {
					return err
				}
				if exists {
					continue
				}

				definition, err := getBunColumnDef(tx, settingsType, column.field)
				if err != nil {
					return fmt.Errorf("error making column definition: %w", err)
				}
				if _, err := tx.NewAddColumn().
					Model(settings).
					ColumnExpr(definition).
					Exec(ctx); err != nil {
					return fmt.Errorf("error adding instance_settings.%s: %w", column.name, err)
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
