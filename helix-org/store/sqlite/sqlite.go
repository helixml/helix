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
	if err := db.AutoMigrate(
		&roleRow{},
		&positionRow{},
		&workerRow{},
		&grantRow{},
		&streamRow{},
		&subscriptionRow{},
		&eventRow{},
		&environmentRow{},
	); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	return &store.Store{
		Roles:         &rolesRepo{db: db},
		Positions:     &positionsRepo{db: db},
		Workers:       &workersRepo{db: db},
		Grants:        &grantsRepo{db: db},
		Streams:       &streamsRepo{db: db},
		Subscriptions: &subscriptionsRepo{db: db},
		Events:        &eventsRepo{db: db},
		Environments:  &environmentsRepo{db: db},
	}, nil
}
