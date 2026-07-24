// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	gtsqlite "code.superseriousbusiness.org/gotosocial/internal/db/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func init() {
	sql.Register("sqlite-hide-quote-fallback-test", &gtsqlite.Driver{})
}

func TestMigrateHideQuoteFallbackUsesConfiguredValue(t *testing.T) {
	original := config.GetStatusesHideQuoteFallback()
	t.Cleanup(func() {
		config.SetStatusesHideQuoteFallback(original)
	})

	for _, value := range []bool{true, false} {
		t.Run(fmt.Sprintf("configured_%t", value), func(t *testing.T) {
			sqlDB, err := sql.Open("sqlite-hide-quote-fallback-test", ":memory:")
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() {
				require.NoError(t, sqlDB.Close())
			})

			db := bun.NewDB(sqlDB, sqlitedialect.New())
			t.Cleanup(func() {
				require.NoError(t, db.Close())
			})

			ctx := context.Background()
			_, err = db.ExecContext(ctx, `CREATE TABLE instance_settings (id TEXT PRIMARY KEY)`)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, `INSERT INTO instance_settings (id) VALUES ('settings')`)
			require.NoError(t, err)

			config.SetStatusesHideQuoteFallback(value)
			require.NoError(t, migrateHideQuoteFallback(ctx, db))

			var stored bool
			require.NoError(t, db.QueryRowContext(
				ctx,
				`SELECT statuses_hide_quote_fallback FROM instance_settings WHERE id = 'settings'`,
			).Scan(&stored))
			require.Equal(t, value, stored)
		})
	}
}
