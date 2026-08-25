// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"database/sql"
	"testing"
	"time"

	gs "code.superseriousbusiness.org/gotosocial/internal/db/sqlite"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func init() {
	sql.Register("sqlite-bluesky-quote-boost-cleanup-test", &gs.Driver{})
}

func TestMigrateBlueskyQuoteBoostCleanup(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite-bluesky-quote-boost-cleanup-test", ":memory:")
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	db := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	_, err = db.ExecContext(ctx, `CREATE TABLE bluesky_connections (
		id CHAR(26) PRIMARY KEY,
		account_id CHAR(26) NOT NULL UNIQUE
	)`)
	require.NoError(t, err)
	require.NoError(t, migrateBlueskyQuoteBoostCleanup(ctx, db))

	want := time.Date(2026, time.August, 21, 22, 0, 0, 0, time.UTC)
	connection := &gtsmodel.BlueskyConnection{
		ID: "01K00000000000000000000000", AccountID: "01K00000000000000000000001",
		QuoteBoostCleanedAt: want,
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO bluesky_connections (id, account_id) VALUES (?, ?)",
		connection.ID, connection.AccountID,
	)
	require.NoError(t, err)
	_, err = db.NewUpdate().
		Model(connection).
		Column("quote_boost_cleaned_at").
		WherePK().
		Exec(ctx)
	require.NoError(t, err)

	var got time.Time
	require.NoError(t, db.NewSelect().
		Table("bluesky_connections").
		Column("quote_boost_cleaned_at").
		Where("id = ?", connection.ID).
		Scan(ctx, &got))
	require.Equal(t, want, got.UTC())
}
