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

	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// orgRowTypes is the canonical list of org-* tables. Kept in one
// place so the FK installation loop stays in sync with AutoMigrate.
var orgRowTypes = []any{
	&roleRow{},
	&workerRow{},
	&reportingLineRow{},
	&workerRuntimeStateRow{},
	&streamRow{},
	&subscriptionRow{},
	&eventRow{},
	&environmentRow{},
	&configRow{},
	&activationRow{},
}

// orgTableNames returns the SQL table names for orgRowTypes. Used by
// the FK installer. The legacy `org_grants` and `org_positions`
// tables are intentionally absent — capability is derived from
// Role.Tools, and reporting is its own org_reporting_lines relation.
// Those tables are no longer migrated; any pre-existing rows are
// simply orphaned (left in place, never read).
var orgTableNames = []string{
	"org_roles",
	"org_workers",
	"org_reporting_lines",
	"org_worker_runtime_state",
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

	if err := db.AutoMigrate(orgRowTypes...); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	if opts.InstallOrganizationFK {
		if err := installOrganizationFKs(db); err != nil {
			return nil, fmt.Errorf("install organization FKs: %w", err)
		}
	}

	// The reporting-line cascade FKs reference org_workers (always
	// migrated above), not organizations, so they install regardless of
	// InstallOrganizationFK — they're what make worker deletion drop
	// reporting lines structurally instead of via app code.
	if err := installReportingLineFKs(db); err != nil {
		return nil, fmt.Errorf("install reporting-line FKs: %w", err)
	}

	workers := newWorkersRepo(db)
	return &store.Store{
		Roles:              newRolesRepo(db),
		Workers:            workers,
		ReportingLines:     newReportingLinesRepo(db),
		WorkerRuntimeState: newWorkerRuntimeStateRepo(db),
		Streams:            newStreamsRepo(db),
		Subscriptions:      newSubscriptionsRepo(db),
		Events:             newEventsRepo(db, workers),
		Environments:       newEnvironmentsRepo(db),
		Configs:            newConfigsRepo(db),
		Activations:        newActivationsRepo(db),
	}, nil
}

// installReportingLineFKs adds the two ON DELETE CASCADE foreign keys
// that make org_reporting_lines self-clean when a Worker is deleted:
// (org_id, manager_id) and (org_id, report_id) both reference
// org_workers(org_id, id). Idempotent — re-adding an existing
// constraint is a no-op. Postgres-specific (our production target);
// unit tests run against the in-memory store and never reach here.
func installReportingLineFKs(db *gorm.DB) error {
	if !db.Migrator().HasTable("org_reporting_lines") || !db.Migrator().HasTable("org_workers") {
		return nil
	}
	type fk struct{ name, cols string }
	for _, f := range []fk{
		{"fk_org_reporting_lines_manager", "org_id, manager_id"},
		{"fk_org_reporting_lines_report", "org_id, report_id"},
	} {
		stmt := fmt.Sprintf(
			`DO $$ BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint WHERE conname = '%s'
				) THEN
					ALTER TABLE org_reporting_lines
						ADD CONSTRAINT %s
						FOREIGN KEY (%s)
						REFERENCES org_workers(org_id, id)
						ON DELETE CASCADE;
				END IF;
			END $$;`,
			f.name, f.name, f.cols,
		)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("add FK %s: %w", f.name, err)
		}
	}
	return nil
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
