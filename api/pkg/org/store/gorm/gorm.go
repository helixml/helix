// Package gorm is the dialect-portable GORM implementation of the
// org store interfaces. Schema row types and repo methods only use
// portable GORM — no dialect-specific hints — so the same code runs
// against any GORM-supported database. In production it is wired to
// helix's existing Postgres connection (see
// api/pkg/server/helix_org.go::openOrgStore); in tests it is wired
// to the shared Postgres test DB via testdb.go::GetOrgTestDB.
package gorm

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/store"
)

// OpenWithDB binds the org-store interfaces against an already-open
// GORM database. Runs AutoMigrate for every row type and returns a
// Store. Callers own the connection lifecycle.
//
// AutoMigrate is idempotent across re-opens and across the GORM
// dialects helix supports.
func OpenWithDB(db *gorm.DB) (*store.Store, error) {
	if db == nil {
		return nil, fmt.Errorf("org-store gorm: db is nil")
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
