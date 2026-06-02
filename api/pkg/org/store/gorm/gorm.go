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

// orgRowTypes is the canonical list of org-* tables. Kept in one
// place so the schema reset + FK installation loops stay in sync
// with AutoMigrate.
var orgRowTypes = []any{
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
}

// orgTableNames returns the SQL table names for orgRowTypes. Used by
// the schema-reset path and the FK installer.
var orgTableNames = []string{
	"org_roles",
	"org_positions",
	"org_workers",
	"org_worker_runtime_state",
	"org_grants",
	"org_streams",
	"org_subscriptions",
	"org_events",
	"org_environments",
	"org_configs",
	"org_activations",
}

// Options controls OpenWithDB behaviour for callers that need to
// opt out of one of the migration steps.
type Options struct {
	// ResetSchema drops every org_* table before AutoMigrate. The
	// composite-PK schema (id, org_id) is not auto-migratable from
	// the prior single-PK shape — production deployments of the alpha
	// pass true on the first deploy of the multi-tenant schema and
	// false thereafter. Tests always pass true so per-test schemas
	// land in a known-good state.
	ResetSchema bool

	// InstallOrganizationFK adds `org_id REFERENCES organizations(id)
	// ON DELETE CASCADE` to every org_* table. Skipped when the
	// `organizations` table doesn't exist in the current search_path
	// (e.g. unit tests against an isolated schema with no helix-proper
	// tables).
	InstallOrganizationFK bool
}

// OpenWithDB binds the org-store interfaces against an already-open
// GORM database. Runs AutoMigrate for every row type and returns a
// Store. Callers own the connection lifecycle.
func OpenWithDB(db *gorm.DB, opts Options) (*store.Store, error) {
	if db == nil {
		return nil, fmt.Errorf("org-store gorm: db is nil")
	}

	if opts.ResetSchema {
		// Drop in reverse-dependency order so FK constraints (when
		// present) don't block individual drops. Postgres DROP TABLE
		// IF EXISTS … CASCADE handles the rest.
		for i := len(orgTableNames) - 1; i >= 0; i-- {
			if err := db.Migrator().DropTable(orgTableNames[i]); err != nil {
				return nil, fmt.Errorf("drop %s: %w", orgTableNames[i], err)
			}
		}
	}

	if err := db.AutoMigrate(orgRowTypes...); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	if opts.InstallOrganizationFK {
		if err := installOrganizationFKs(db); err != nil {
			return nil, fmt.Errorf("install organization FKs: %w", err)
		}
	}

	return &store.Store{
		Roles:              newRolesRepo(db),
		Positions:          newPositionsRepo(db),
		Workers:            newWorkersRepo(db),
		WorkerRuntimeState: &workerRuntimeStateRepo{db: db},
		Grants:             newGrantsRepo(db),
		Streams:            newStreamsRepo(db),
		Subscriptions:      newSubscriptionsRepo(db),
		Events:             newEventsRepo(db),
		Environments:       newEnvironmentsRepo(db),
		Configs:            newConfigsRepo(db),
		Activations:        newActivationsRepo(db),
	}, nil
}

// installOrganizationFKs adds the FK constraint
// `org_id REFERENCES organizations(id) ON DELETE CASCADE` on every
// org_* table. Idempotent — re-adding an existing constraint is a
// no-op. If the `organizations` table doesn't exist (test schemas),
// the function returns nil so callers don't have to know about that.
func installOrganizationFKs(db *gorm.DB) error {
	// Check the organizations table exists in the current search_path
	// — otherwise FK creation fails and there's nothing useful to do.
	if !db.Migrator().HasTable("organizations") {
		return nil
	}

	for _, table := range orgTableNames {
		constraint := fmt.Sprintf("fk_%s_org", table)
		// Postgres-specific syntax. Dialect-portable equivalents exist
		// but our production target IS Postgres; tests skip this path
		// when no organizations table is present.
		stmt := fmt.Sprintf(
			`DO $$ BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint WHERE conname = '%s'
				) THEN
					ALTER TABLE %s
						ADD CONSTRAINT %s
						FOREIGN KEY (org_id)
						REFERENCES organizations(id)
						ON DELETE CASCADE;
				END IF;
			END $$;`,
			constraint, table, constraint,
		)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("add FK %s: %w", constraint, err)
		}
	}
	return nil
}
