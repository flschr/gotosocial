// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"fmt"
	"reflect"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/uptrace/bun"
)

func init() {
	up := func(ctx context.Context, db *bun.DB) error {
		return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			var settings *gtsmodel.InstanceSettings
			exists, err := doesColumnExist(ctx, tx, "instance_settings", "statuses_hide_quote_fallback")
			if err != nil {
				return err
			}
			if !exists {
				definition, err := getBunColumnDef(
					tx,
					reflect.TypeOf(settings),
					"StatusesHideQuoteFallback",
				)
				if err != nil {
					return fmt.Errorf("error making column definition: %w", err)
				}
				if _, err := tx.NewAddColumn().
					Model(settings).
					ColumnExpr(definition).
					Exec(ctx); err != nil {
					return fmt.Errorf(
						"error adding instance_settings.statuses_hide_quote_fallback: %w",
						err,
					)
				}
			}

			_, err = tx.NewUpdate().
				Table("instance_settings").
				Set("statuses_hide_quote_fallback = ?", config.GetStatusesHideQuoteFallback()).
				Where("1 = 1").
				Exec(ctx)
			return err
		})
	}

	down := func(context.Context, *bun.DB) error { return nil }

	if err := Migrations.Register(up, down); err != nil {
		panic(err)
	}
}
