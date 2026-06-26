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
	&topicRow{},
	&subscriptionRow{},
	&eventRow{},
	&configRow{},
	&activationRow{},
	&processorRow{},
	&domainEventRow{},
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
	"org_topics",
	"org_subscriptions",
	"org_events",
	"org_configs",
	"org_activations",
	"org_processors",
	"org_domain_events",
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

	// Rename legacy tables before AutoMigrate. The Stream→Topic rename
	// (design/2026-06-18-helix-org-topics-processors.md, Phase 0)
	// renamed the row's TableName() from org_streams to org_topics. On a
	// DB that predates the rename AutoMigrate would otherwise CREATE a
	// fresh empty org_topics and orphan the existing org_streams rows.
	// Renaming first preserves the data; idempotent (only fires when the
	// old table exists and the new one does not).
	if err := renameLegacyTables(db); err != nil {
		return nil, fmt.Errorf("rename legacy tables: %w", err)
	}

	if err := db.AutoMigrate(orgRowTypes...); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}

	// Drop tables for aggregates removed from the model. AutoMigrate never
	// drops, so a DB migrated before an aggregate was deleted keeps the
	// orphaned table forever. org_environments backed the Environment
	// aggregate (the on-disk per-Worker env dir), removed when worker
	// config moved to the per-Worker helix-specs branch. Dropping it
	// idempotently lets fresh and upgraded DBs converge. No org_* table
	// FKs reference it, so the drop is safe.
	if err := dropRemovedTables(db); err != nil {
		return nil, fmt.Errorf("drop removed tables: %w", err)
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
		Topics:            newTopicsRepo(db),
		Subscriptions:      newSubscriptionsRepo(db),
		Events:             newEventsRepo(db, workers),
		Configs:            newConfigsRepo(db),
		Activations:        newActivationsRepo(db),
		Processors:         newProcessorsRepo(db),
		DomainEvents:       newDomainEventsRepo(db),
	}, nil
}

// renamedTables maps an old table name to its current name for
// behaviour-preserving renames. Applied before AutoMigrate so an
// upgraded DB carries its data into the renamed table instead of
// AutoMigrate creating an empty one alongside the orphaned original.
var renamedTables = []struct{ from, to string }{
	{from: "org_streams", to: "org_topics"}, // Stream→Topic rename (Phase 0)
}

// renamedColumns maps a (table, oldColumn) to its current column name.
// The Stream→Topic rename renamed the StreamID field to TopicID, which
// GORM maps to a stream_id→topic_id column rename. AutoMigrate only
// ADDS columns, so without an explicit rename it would add a NOT NULL
// topic_id alongside the populated stream_id and fail on existing rows.
var renamedColumns = []struct{ table, from, to string }{
	{table: "org_events", from: "stream_id", to: "topic_id"},
	{table: "org_subscriptions", from: "stream_id", to: "topic_id"},
}

// renamedIndexes maps a (table, oldIndex) to its current index name so
// the renamed unique index is carried over instead of orphaned.
var renamedIndexes = []struct{ table, from, to string }{
	{table: "org_topics", from: "idx_stream_org_name", to: "idx_topic_org_name"},
}

// renameLegacyTables applies renamedTables, renamedColumns and
// renamedIndexes before AutoMigrate. Each step is guarded so the whole
// function is idempotent: a no-op on a fresh DB (old names absent) and
// on an already-migrated DB (new names present).
func renameLegacyTables(db *gorm.DB) error {
	m := db.Migrator()
	for _, r := range renamedTables {
		if m.HasTable(r.from) && !m.HasTable(r.to) {
			if err := m.RenameTable(r.from, r.to); err != nil {
				return fmt.Errorf("rename table %s -> %s: %w", r.from, r.to, err)
			}
		}
	}
	for _, c := range renamedColumns {
		if m.HasTable(c.table) && m.HasColumn(c.table, c.from) && !m.HasColumn(c.table, c.to) {
			if err := db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", c.table, c.from, c.to)).Error; err != nil {
				return fmt.Errorf("rename column %s.%s -> %s: %w", c.table, c.from, c.to, err)
			}
		}
	}
	for _, i := range renamedIndexes {
		if m.HasTable(i.table) && m.HasIndex(i.table, i.from) && !m.HasIndex(i.table, i.to) {
			if err := db.Exec(fmt.Sprintf("ALTER INDEX %s RENAME TO %s", i.from, i.to)).Error; err != nil {
				return fmt.Errorf("rename index %s -> %s: %w", i.from, i.to, err)
			}
		}
	}
	return nil
}

// removedTables names tables for aggregates deleted from the model.
// AutoMigrate never drops, so these are dropped explicitly on open.
var removedTables = []string{
	"org_environments", // Environment aggregate; config moved to helix-specs branch
}

// dropRemovedTables drops the removedTables if present. Idempotent
// (DROP TABLE IF EXISTS); a no-op on a fresh DB that never created them.
func dropRemovedTables(db *gorm.DB) error {
	for _, t := range removedTables {
		if err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", t)).Error; err != nil {
			return fmt.Errorf("drop %s: %w", t, err)
		}
	}
	return nil
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
