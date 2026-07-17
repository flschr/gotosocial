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
			settingsType := reflect.TypeOf(settings)

			for _, column := range []struct {
				name  string
				field string
			}{
				{"accounts_use_account_domain_in_acct", "AccountsUseAccountDomainInAcct"},
				{"accounts_hide_local_roles", "AccountsHideLocalRoles"},
				{"statuses_preview_cards", "StatusesPreviewCards"},
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

			// Preserve the configured YAML values as the initial UI values.
			_, err := tx.NewUpdate().
				Table("instance_settings").
				Set("accounts_use_account_domain_in_acct = ?", config.GetAccountsUseAccountDomainInAcct()).
				Set("accounts_hide_local_roles = ?", config.GetAccountsHideLocalRoles()).
				Set("statuses_preview_cards = ?", config.GetStatusesPreviewCards()).
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
