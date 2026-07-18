// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package bundb

import (
	"context"
	"database/sql"
	"fmt"
	"runtime"
	"strings"

	"code.superseriousbusiness.org/gopkg/log"
	"code.superseriousbusiness.org/gotosocial/internal/config"
	"code.superseriousbusiness.org/gotosocial/internal/db"
	"code.superseriousbusiness.org/gotosocial/internal/db/bundb/migrations"
	"code.superseriousbusiness.org/gotosocial/internal/gtserror"
	"code.superseriousbusiness.org/gotosocial/internal/gtsmodel"
	"code.superseriousbusiness.org/gotosocial/internal/observability"
	"code.superseriousbusiness.org/gotosocial/internal/state"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/migrate"
	"github.com/uptrace/bun/schema"
)

// DBService satisfies the DB interface
type DBService struct {
	db.Account
	db.Admin
	db.AdvancedMigration
	db.Application
	db.Basic
	db.Bluesky
	db.Conversation
	db.Directory
	db.Domain
	db.Emoji
	db.HeaderFilter
	db.Instance
	db.Interaction
	db.Filter
	db.List
	db.Marker
	db.Media
	db.Mention
	db.Move
	db.Notification
	db.Poll
	db.Relationship
	db.Relay
	db.Report
	db.Rule
	db.ScheduledStatus
	db.Search
	db.Session
	db.SinBinStatus
	db.Status
	db.StatusBookmark
	db.StatusEdit
	db.StatusFave
	db.StatusPin
	db.Tag
	db.Thread
	db.Timeline
	db.User
	db.Tombstone
	db.WebPush
	db.WorkerTask
	db *bun.DB
}

// GetDB returns the underlying database connection pool.
// Should only be used in testing + exceptional circumstance.
func (dbService *DBService) DB() *bun.DB {
	return dbService.db
}

func doMigration(ctx context.Context, db *bun.DB) error {
	migrator := migrate.NewMigrator(db, migrations.Migrations)

	if err := migrator.Init(ctx); err != nil {
		return err
	}

	group, err := migrator.Migrate(ctx)
	if err != nil && !strings.Contains(err.Error(), "no migrations") {
		return err
	}

	if group == nil || group.ID == 0 {
		log.Info(ctx, "there are no new migrations to run")
		return nil
	}

	log.Infof(ctx, "MIGRATED DATABASE TO %s", group)

	if db.Dialect().Name() == dialect.SQLite {
		// Perform a final WAL checkpoint after a migration on SQLite.
		if strings.EqualFold(config.GetDbSqliteJournalMode(), "WAL") {
			_, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(RESTART);")
			if err != nil {
				return gtserror.Newf("error performing wal_checkpoint: %w", err)
			}
		}

		log.Info(ctx, "running ANALYZE to update table and index statistics; this will take somewhere between "+
			"1-10 minutes, or maybe longer depending on your hardware and database size, please be patient")
		_, err := db.ExecContext(ctx, "ANALYZE;")
		if err != nil {
			log.Warnf(ctx, "ANALYZE failed, query planner may make poor life choices: %s", err)
		}
	}
	return nil
}

// NewBunDBService returns a bunDB derived from the provided config, which implements the go-fed DB interface.
// Under the hood, it uses https://github.com/uptrace/bun to create and maintain a database connection.
func NewBunDBService(ctx context.Context, state *state.State) (db.DB, error) {
	var sqldb *sql.DB
	var dialect func() schema.Dialect
	var err error

	switch t := strings.ToLower(config.GetDbType()); t {
	case "postgres":
		sqldb, dialect, err = pgConn(ctx)
		if err != nil {
			return nil, err
		}
	case "sqlite":
		sqldb, dialect, err = sqliteConn(ctx)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("database type %s not supported for bundb", t)
	}

	// perform any pending database migrations: this includes the first
	// 'migration' on startup which just creates necessary db tables.
	//
	// Note this uses its own instance of bun.DB as bun will automatically
	// store in-memory reflect type schema of any Go models passed to it,
	// and we still maintain lots of old model versions in the migrations.
	if err := doMigration(ctx, bunDB(sqldb, dialect)); err != nil {
		return nil, fmt.Errorf("db migration error: %s", err)
	}

	// Wrap sql.DB as bun.DB type,
	// adding any connection hooks.
	db := bunDB(sqldb, dialect)

	ps := &DBService{
		Account: &accountDB{
			db:    db,
			state: state,
		},
		Admin: &adminDB{
			db:    db,
			state: state,
		},
		AdvancedMigration: &advancedMigrationDB{
			db:    db,
			state: state,
		},
		Application: &applicationDB{
			db:    db,
			state: state,
		},
		Basic: &basicDB{
			db: db,
		},
		Bluesky: &blueskyDB{
			db: db, state: state,
		},
		Conversation: &conversationDB{
			db:    db,
			state: state,
		},
		Directory: &directoryDB{
			db:    db,
			state: state,
		},
		Domain: &domainDB{
			db:    db,
			state: state,
		},
		Emoji: &emojiDB{
			db:    db,
			state: state,
		},
		HeaderFilter: &headerFilterDB{
			db:    db,
			state: state,
		},
		Instance: &instanceDB{
			db:    db,
			state: state,
		},
		Interaction: &interactionDB{
			db:    db,
			state: state,
		},
		Filter: &filterDB{
			db:    db,
			state: state,
		},
		List: &listDB{
			db:    db,
			state: state,
		},
		Marker: &markerDB{
			db:    db,
			state: state,
		},
		Media: &mediaDB{
			db:    db,
			state: state,
		},
		Mention: &mentionDB{
			db:    db,
			state: state,
		},
		Move: &moveDB{
			db:    db,
			state: state,
		},
		Notification: &notificationDB{
			db:    db,
			state: state,
		},
		Poll: &pollDB{
			db:    db,
			state: state,
		},
		Relationship: &relationshipDB{
			db:    db,
			state: state,
		},
		Relay: &relayDB{
			db:    db,
			state: state,
		},
		Report: &reportDB{
			db:    db,
			state: state,
		},
		Rule: &ruleDB{
			db:    db,
			state: state,
		},
		ScheduledStatus: &scheduledStatusDB{
			db:    db,
			state: state,
		},
		Search: &searchDB{
			db:    db,
			state: state,
		},
		Session: &sessionDB{
			db: db,
		},
		SinBinStatus: &sinBinStatusDB{
			db:    db,
			state: state,
		},
		Status: &statusDB{
			db:    db,
			state: state,
		},
		StatusBookmark: &statusBookmarkDB{
			db:    db,
			state: state,
		},
		StatusEdit: &statusEditDB{
			db:    db,
			state: state,
		},
		StatusFave: &statusFaveDB{
			db:    db,
			state: state,
		},
		StatusPin: &statusPinDB{
			db:    db,
			state: state,
		},
		Tag: &tagDB{
			db:    db,
			state: state,
		},
		Thread: &threadDB{
			db:    db,
			state: state,
		},
		Timeline: &timelineDB{
			db:    db,
			state: state,
		},
		User: &userDB{
			db:    db,
			state: state,
		},
		Tombstone: &tombstoneDB{
			db:    db,
			state: state,
		},
		WebPush: &webPushDB{
			db:    db,
			state: state,
		},
		WorkerTask: &workerTaskDB{
			db: db,
		},
		db: db,
	}

	// we can confidently return this useable service now
	return ps, nil
}

// bunDB returns a new bun.DB for given sql.DB connection pool and dialect
// function. This can be used to apply any necessary opts / hooks as we
// initialize a bun.DB object both before and after performing migrations.
func bunDB(sqldb *sql.DB, dialect func() schema.Dialect) *bun.DB {
	db := bun.NewDB(sqldb, dialect())

	// Add SQL logging hooks depending on current configuration.
	if config.GetLogDbQueries() || log.Level() <= log.TRACE {
		db.AddQueryHook(queryLogger{})
	} else {
		db.AddQueryHook(slowQueryLogger{})
	}

	// Add any SQL observability hooks if enabled.
	metricsEnabled := config.GetMetricsEnabled()
	tracingEnabled := config.GetTracingEnabled()
	if metricsEnabled || tracingEnabled {
		db.AddQueryHook(observability.InstrumentBun(tracingEnabled, metricsEnabled))
	}

	// table registration is needed for many-to-many, see:
	// https://bun.uptrace.dev/orm/many-to-many-relation/
	for _, t := range []interface{}{
		&gtsmodel.AccountToEmoji{},
		&gtsmodel.ConversationToStatus{},
		&gtsmodel.StatusToEmoji{},
		&gtsmodel.StatusToTag{},
	} {
		db.RegisterModel(t)
	}

	return db
}

/*
	HANDY STUFF
*/

// maxOpenConns returns multiplier * GOMAXPROCS,
// returning just 1 instead if multiplier < 1.
func maxOpenConns() int {
	multiplier := config.GetDbMaxOpenConnsMultiplier()
	if multiplier < 1 {
		return 1
	}

	// Specifically for SQLite databases with
	// a journal mode of anything EXCEPT "wal",
	// only 1 concurrent connection is supported.
	if strings.ToLower(config.GetDbType()) == "sqlite" {
		journalMode := config.GetDbSqliteJournalMode()
		journalMode = strings.ToLower(journalMode)
		if journalMode != "wal" {
			return 1
		}
	}

	return multiplier * runtime.GOMAXPROCS(0)
}
