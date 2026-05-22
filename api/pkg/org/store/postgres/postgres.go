// Package postgres wraps an already-open Postgres GORM database in
// the org-store interfaces. Schema models and repo implementations
// live in the sibling sqlite package — both dialects share the
// same dialect-portable GORM code. Only the dialect-opening step
// differs.
//
// In production helix's PostgresStore exposes its *gorm.DB via
// (*PostgresStore).GormDB(); the embedded helix-org call site
// (api/pkg/server/helix_org.go) calls postgres.Open on that DB to
// land org-graph rows in the same Postgres database that holds
// helix's primary state. That removes the legacy
// FILESTORE_TYPE=fs requirement — gcs/s3 deployments can run the
// alpha because the org store no longer needs a local SQLite file.
//
// Wiring lands in H4.4; this package exposes the constructor so
// the wiring change is a one-line swap.
package postgres

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
)

// Open binds the org-store interfaces against an already-open
// Postgres GORM connection. Delegates to sqlite.OpenWithDB for the
// schema migration + repo wiring — every row type and repo method
// is dialect-portable GORM.
//
// db must be non-nil. AutoMigrate creates / updates the org-graph
// tables in the same database the *gorm.DB points at; idempotent so
// every helix boot can safely call it.
func Open(db *gorm.DB) (*store.Store, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres org-store: db is nil")
	}
	return sqlite.OpenWithDB(db)
}
