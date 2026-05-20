// Package sqlite is the GORM/SQLite implementation of the store interfaces.
package sqlite

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/helixml/helix-org/store"
)

// Open opens a SQLite database at the given path (use ":memory:" for tests)
// and runs AutoMigrate. It returns a Store bound to the concrete repos.
//
// For ":memory:" DSNs, the connection pool is pinned to a single
// connection. Without this, every new connection in the pool gets its
// own private in-memory database — concurrent HTTP requests would
// each see a different (empty) DB. File-backed DSNs are unaffected.
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
	}, nil
}
