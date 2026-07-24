// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"code.superseriousbusiness.org/gotosocial/internal/config"
	gtsqlite "code.superseriousbusiness.org/gotosocial/internal/db/sqlite"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
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

	t.Run("sqlite", func(t *testing.T) {
		sqlDB, err := sql.Open("sqlite-hide-quote-fallback-test", ":memory:")
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		db := bun.NewDB(sqlDB, sqlitedialect.New())
		t.Cleanup(func() {
			require.NoError(t, db.Close())
		})

		testMigrateHideQuoteFallbackConfiguredValues(t, db)
	})

	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("GTS_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("GTS_TEST_POSTGRES_DSN is not configured")
		}

		config, err := pgx.ParseConfig(dsn)
		require.NoError(t, err)
		sqlDB := stdlib.OpenDB(*config)
		db := bun.NewDB(sqlDB, pgdialect.New())
		t.Cleanup(func() {
			require.NoError(t, db.Close())
		})

		testMigrateHideQuoteFallbackConfiguredValues(t, db)
	})
}

func testMigrateHideQuoteFallbackConfiguredValues(t *testing.T, db *bun.DB) {
	for _, value := range []bool{true, false} {
		t.Run(fmt.Sprintf("configured_%t", value), func(t *testing.T) {
			ctx := context.Background()
			_, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS instance_settings`)
			require.NoError(t, err)
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
