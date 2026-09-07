// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package migrations

import (
	"context"
	"database/sql"
	"testing"

	gs "code.superseriousbusiness.org/gotosocial/internal/db/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
)

func init() {
	sql.Register("sqlite-bluesky-app-password-test", &gs.Driver{})
}

func TestMigrateBlueskyAppPassword(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := sql.Open("sqlite-bluesky-app-password-test", ":memory:")
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database := bun.NewDB(sqlDB, sqlitedialect.New())
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	_, err = database.ExecContext(ctx, `CREATE TABLE bluesky_connections (
		id CHAR(26) PRIMARY KEY,
		account_id CHAR(26) NOT NULL UNIQUE
	)`)
	require.NoError(t, err)
	require.NoError(t, migrateBlueskyAppPassword(ctx, database))
	_, err = database.ExecContext(ctx,
		"INSERT INTO bluesky_connections (id, account_id, app_password_data) VALUES (?, ?, ?)",
		"01K00000000000000000000000", "01K00000000000000000000001", []byte("encrypted"),
	)
	require.NoError(t, err)

	var stored []byte
	require.NoError(t, database.NewSelect().
		Table("bluesky_connections").
		Column("app_password_data").
		Where("id = ?", "01K00000000000000000000000").
		Scan(ctx, &stored))
	require.Equal(t, []byte("encrypted"), stored)
}
