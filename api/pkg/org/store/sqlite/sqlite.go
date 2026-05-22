// Package sqlite is the GORM/SQLite implementation of the store
// interfaces. The schema (rows + AutoMigrate sequence) and the repo
// implementations are dialect-portable GORM — only the dialect-
// opening logic in Open is sqlite-specific. The Postgres impl
// (H4.3) wraps an existing *gorm.DB and calls OpenWithDB to share
// the row types and repos.
package sqlite

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/helixml/helix/api/pkg/org/store"
)

// Open opens a SQLite database at the given path (use ":memory:" for tests)
// and returns a Store bound to the concrete repos.
//
// For ":memory:" DSNs, the connection pool is pinned to a single
// connection. Without this, every new connection in the pool gets its
// own private in-memory database — concurrent HTTP requests would
// each see a different (empty) DB. File-backed DSNs are unaffected.
//
// The AutoMigrate + repo-wiring path is shared with OpenWithDB so
// non-sqlite callers (Postgres, future dialects) reuse the same
// schema.
func Open(dsn string) (*store.Store, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dsn, err)
	}
	if dsn == ":memory:" {
		sqlDB, err := db.DB()
		if err != nil {
			return nil, fmt.Errorf("get sql.DB: %w", err)
		}
		sqlDB.SetMaxOpenConns(1)
	}
	return OpenWithDB(db)
}

// OpenWithDB binds the org-store interfaces against an already-open
// GORM database. Runs AutoMigrate for every row type and returns a
// Store. Callers with their own *gorm.DB (e.g. the helix Postgres
// connection, H4.3) use this to skip Open's dialect-specific
// connection setup.
//
// The AutoMigrate sequence is portable across the GORM dialects
// helix supports (sqlite, postgres) — no dialect-specific schema
// hints are used. If a future dialect requires special-casing,
// extend by branching inside this function rather than fanning out
// duplicate AutoMigrate calls in every caller.
func OpenWithDB(db *gorm.DB) (*store.Store, error) {
	if err := db.AutoMigrate(
		&roleRow{},
		&positionRow{},
		&workerRow{},
		&workerRuntimeStateRow{},
		&grantRow{},
		&streamRow{},
		&subscriptionRow{},
		&eventRow{},
		&environmentRow{},
		&configRow{},
		&activationRow{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	return &store.Store{
		Roles:              &rolesRepo{db: db},
		Positions:          &positionsRepo{db: db},
		Workers:            &workersRepo{db: db},
		WorkerRuntimeState: &workerRuntimeStateRepo{db: db},
		Grants:             &grantsRepo{db: db},
		Streams:            &streamsRepo{db: db},
		Subscriptions:      &subscriptionsRepo{db: db},
		Events:             &eventsRepo{db: db},
		Environments:       &environmentsRepo{db: db},
		Configs:            &configsRepo{db: db},
		Activations:        &activationsRepo{db: db},
	}, nil
}
