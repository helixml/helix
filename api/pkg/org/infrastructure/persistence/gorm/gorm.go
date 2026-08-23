// Package gorm is the dialect-portable GORM implementation of the
// org store interfaces. Schema row types and repo methods only use
// portable GORM — no dialect-specific hints — so the same code runs
// against any GORM-supported database. In production it is wired to
// helix's existing Postgres connection (see
// api/pkg/server/helix_org.go::openOrgStore); in tests it is wired
// to the shared Postgres test DB via testdb.go::GetOrgTestDB.
package gorm

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/types"
)

// orgRowTypes is the canonical list of org-* tables. Kept in one
// place so the FK installation loop stays in sync with AutoMigrate.
var orgRowTypes = []any{
	&nodeRow{},
	&reportingLineRow{},
	&nodeRuntimeStateRow{},
	&eventRow{},
	&configRow{},
	&activationRow{},
	&processorRow{},
	&triggerRow{},
	&attachmentRow{},
	&workerSecretBindingRow{},
	&asset.Asset{},
	&asset.Link{},
	&chartPositionRow{},
	&domainEventRow{},
}

// orgTableNames returns the SQL table names for orgRowTypes. Used by
// the FK installer. The legacy `org_grants`, `org_positions`,
// `org_roles` and `org_workers` tables are intentionally absent — the
// former Role and Worker collapsed into the single `org_bots` table,
// capability is derived from Bot.Tools, and reporting is its own
// org_reporting_lines relation. Those tables are no longer migrated;
// they are dropped by migration 0005.
var orgTableNames = []string{
	"org_bots",
	"org_reporting_lines",
	"org_bot_runtime_state",
	"org_events",
	"org_configs",
	"org_activations",
	"org_processors",
	"org_triggers",
	"org_worker_attachments",
	"org_worker_secret_bindings",
	"org_assets",
	"org_asset_links",
	"org_chart_positions",
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
	if err := renameLegacyAssetLinkAgentColumn(db); err != nil {
		return nil, fmt.Errorf("rename legacy asset-link agent column: %w", err)
	}

	if err := db.AutoMigrate(orgRowTypes...); err != nil {
		return nil, fmt.Errorf("auto-migrate: %w", err)
	}
	if err := migrateProcessorOutputIDs(db); err != nil {
		return nil, fmt.Errorf("migrate processor output ids: %w", err)
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

	// The reporting-line cascade FKs reference org_bots (always
	// migrated above), not organizations, so they install regardless of
	// InstallOrganizationFK — they're what make bot deletion drop
	// reporting lines structurally instead of via app code.
	if err := installReportingLineFKs(db); err != nil {
		return nil, fmt.Errorf("install reporting-line FKs: %w", err)
	}
	if err := installAssetLinkFKs(db); err != nil {
		return nil, fmt.Errorf("install asset-link FKs: %w", err)
	}
	if err := installAttachmentConstraints(db); err != nil {
		return nil, fmt.Errorf("install attachment constraints: %w", err)
	}
	if err := installWorkerSecretConstraints(db); err != nil {
		return nil, fmt.Errorf("install worker secret constraints: %w", err)
	}
	if err := installAgentAppLinks(db); err != nil {
		return nil, fmt.Errorf("install agent app links: %w", err)
	}

	bots := newNodesRepo(db)
	return &store.Store{
		Nodes:                bots,
		ReportingLines:       newReportingLinesRepo(db),
		NodeRuntimeState:     newNodeRuntimeStateRepo(db),
		Events:               newEventsRepo(db),
		Configs:              newConfigsRepo(db),
		Activations:          newActivationsRepo(db),
		Processors:           newProcessorsRepo(db),
		Triggers:             newTriggersRepo(db),
		WorkerAttachments:    newAttachmentsRepo(db),
		WorkerSecretBindings: &workerSecretBindingsRepo{db: db},
		Assets:               newAssetsRepo(db),
		AssetLinks:           newAssetLinksRepo(db),
		ChartPositions:       newChartPositionsRepo(db),
		DomainEvents:         newDomainEventsRepo(db),

		RetiredTopics:          newRetiredReader(db),
		RetiredSubscriptions:   newRetiredSubscriptionReader(db),
		RetiredProcessorInputs: newRetiredProcessorInputReader(db),
	}, nil
}

func installWorkerSecretConstraints(db *gorm.DB) error {
	if !db.Migrator().HasTable("org_worker_secret_bindings") || db.Dialector.Name() != "postgres" {
		return nil
	}
	statements := []string{`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_worker_secret_source') THEN ALTER TABLE org_worker_secret_bindings ADD CONSTRAINT chk_worker_secret_source CHECK ((source_kind='helix_secret' AND secret_id<>'' AND account_id='' AND export_key='') OR (source_kind='connected_account' AND secret_id='' AND account_id<>'' AND export_key<>'')); END IF; END $$;`}
	if db.Migrator().HasTable("org_bots") {
		statements = append(statements, `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_worker_secret_worker') THEN ALTER TABLE org_worker_secret_bindings ADD CONSTRAINT fk_worker_secret_worker FOREIGN KEY (org_id,worker_id) REFERENCES org_bots(org_id,id) ON DELETE CASCADE; END IF; END $$;`)
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func installAttachmentConstraints(db *gorm.DB) error {
	if !db.Migrator().HasTable("org_worker_attachments") {
		return nil
	}
	statements := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_attachment_trigger ON org_worker_attachments (org_id, worker_id, trigger_id) WHERE trigger_id IS NOT NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_attachment_processor_output ON org_worker_attachments (org_id, worker_id, processor_id, output_id) WHERE processor_id IS NOT NULL`,
	}
	if db.Dialector.Name() != "postgres" {
		for _, stmt := range statements {
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
		return nil
	}
	statements = append(statements, `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'chk_attachment_source') THEN ALTER TABLE org_worker_attachments ADD CONSTRAINT chk_attachment_source CHECK ((trigger_id IS NOT NULL AND processor_id IS NULL AND output_id IS NULL) OR (trigger_id IS NULL AND processor_id IS NOT NULL AND output_id IS NOT NULL)); END IF; END $$;`)
	if db.Migrator().HasTable("org_bots") {
		statements = append(statements, `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_attachment_worker') THEN ALTER TABLE org_worker_attachments ADD CONSTRAINT fk_attachment_worker FOREIGN KEY (org_id, worker_id) REFERENCES org_bots(org_id, id) ON DELETE CASCADE; END IF; END $$;`)
	}
	if db.Migrator().HasTable("org_triggers") {
		statements = append(statements, `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_attachment_trigger') THEN ALTER TABLE org_worker_attachments ADD CONSTRAINT fk_attachment_trigger FOREIGN KEY (org_id, trigger_id) REFERENCES org_triggers(org_id, id) ON DELETE CASCADE; END IF; END $$;`)
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateProcessorOutputIDs(db *gorm.DB) error {
	var rows []processorRow
	if err := db.Where(`outputs NOT LIKE ?`, `%"id":%`).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var outputs []processor.Output
		if err := json.Unmarshal([]byte(row.Outputs), &outputs); err != nil {
			return fmt.Errorf("processor %q outputs: %w", row.ID, err)
		}
		changed := false
		seen := map[string]struct{}{}
		for i := range outputs {
			if outputs[i].ID == "" {
				if outputs[i].StreamID == "" {
					return fmt.Errorf("processor %q output %d has no topic id", row.ID, i)
				}
				outputs[i].ID = processor.LegacyOutputID(outputs[i].StreamID)
				changed = true
			}
			if _, ok := seen[outputs[i].ID]; ok {
				return fmt.Errorf("processor %q has duplicate output id %q", row.ID, outputs[i].ID)
			}
			seen[outputs[i].ID] = struct{}{}
		}
		if changed {
			encoded, err := json.Marshal(outputs)
			if err != nil {
				return err
			}
			if err := db.Model(&processorRow{}).Where("org_id = ? AND id = ?", row.OrgID, row.ID).Update("outputs", string(encoded)).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func renameLegacyAssetLinkAgentColumn(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("org_asset_links") || !migrator.HasColumn("org_asset_links", "bot_id") {
		return nil
	}
	if migrator.HasColumn("org_asset_links", "agent_id") {
		if migrator.HasConstraint("org_asset_links", "fk_org_asset_links_agent") {
			if err := migrator.DropConstraint(&asset.Link{}, "fk_org_asset_links_agent"); err != nil {
				return fmt.Errorf("drop transitional agent FK: %w", err)
			}
		}
		if err := migrator.DropColumn(&asset.Link{}, "AgentID"); err != nil {
			return fmt.Errorf("drop transitional agent_id column: %w", err)
		}
	}
	if migrator.HasConstraint("org_asset_links", "fk_org_asset_links_bot") {
		if err := migrator.DropConstraint(&asset.Link{}, "fk_org_asset_links_bot"); err != nil {
			return fmt.Errorf("drop legacy bot FK: %w", err)
		}
	}
	if err := migrator.RenameColumn("org_asset_links", "bot_id", "agent_id"); err != nil {
		return fmt.Errorf("rename bot_id to agent_id: %w", err)
	}
	return nil
}

func installAssetLinkFKs(db *gorm.DB) error {
	if !db.Migrator().HasTable("org_asset_links") || !db.Migrator().HasTable("org_assets") || !db.Migrator().HasTable("org_bots") {
		return nil
	}
	type fk struct{ name, cols, target string }
	for _, f := range []fk{
		{"fk_org_asset_links_asset", "org_id, asset_id", "org_assets(org_id, id)"},
		{"fk_org_asset_links_agent", "org_id, agent_id", "org_bots(org_id, id)"},
	} {
		stmt := fmt.Sprintf(`DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = '%s') THEN
				ALTER TABLE org_asset_links ADD CONSTRAINT %s
				FOREIGN KEY (%s) REFERENCES %s ON DELETE CASCADE;
			END IF;
		END $$;`, f.name, f.name, f.cols, f.target)
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("add FK %s: %w", f.name, err)
		}
	}
	return nil
}

func installAgentAppLinks(db *gorm.DB) error {
	if !db.Migrator().HasTable("org_bots") || !db.Migrator().HasTable("apps") {
		return nil
	}
	if db.Migrator().HasTable("org_bot_runtime_state") {
		if err := backfillAgentAppLinks(db); err != nil {
			return err
		}
	}
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_org_bots_agent_app
		ON org_bots (org_id, agent_app_id)
		WHERE agent_app_id IS NOT NULL
	`).Error; err != nil {
		return fmt.Errorf("add unique index: %w", err)
	}
	if err := db.Exec(`
		DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'fk_org_bots_agent_app'
			) THEN
				ALTER TABLE org_bots
					ADD CONSTRAINT fk_org_bots_agent_app
					FOREIGN KEY (agent_app_id)
					REFERENCES apps(id)
					ON DELETE RESTRICT;
			END IF;
		END $$;
	`).Error; err != nil {
		return fmt.Errorf("add foreign key: %w", err)
	}
	if err := backfillProjectAgentApps(db); err != nil {
		return err
	}
	if err := db.Exec(`
		UPDATE apps AS app
		SET agent_kind = ?
		FROM org_bots AS bot
		WHERE bot.agent_app_id = app.id
		  AND bot.agent_app_id IS NOT NULL
	`, types.AgentKindOrg).Error; err != nil {
		return fmt.Errorf("backfill org agent kinds: %w", err)
	}
	return nil
}

func backfillAgentAppLinks(db *gorm.DB) error {
	type candidate struct {
		OrgID  string
		NodeID string
		AppID  string
		Config string
	}
	var candidates []candidate
	if err := db.Raw(`
		SELECT state.org_id, state.bot_id AS node_id, state.value AS app_id,
		       CAST(a.config AS TEXT) AS config
		FROM org_bot_runtime_state AS state
		JOIN org_bots AS bot
		  ON bot.org_id = state.org_id
		 AND bot.id = state.bot_id
		JOIN apps AS a
		  ON a.id = state.value
		 AND a.organization_id = state.org_id
		WHERE bot.kind <> 'human'
		  AND bot.agent_app_id IS NULL
		  AND state.backend = 'helix'
		  AND state.key = 'agent_app_id'
		  AND state.value <> ''
		  AND NOT EXISTS (
			SELECT 1
			FROM org_bot_runtime_state AS other
			WHERE other.org_id = state.org_id
			  AND other.backend = state.backend
			  AND other.key = state.key
			  AND other.value = state.value
			  AND other.bot_id <> state.bot_id
		  )
	`).Scan(&candidates).Error; err != nil {
		return fmt.Errorf("list agent link candidates: %w", err)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, candidate := range candidates {
			var config types.AppConfig
			if json.Unmarshal([]byte(candidate.Config), &config) != nil || len(config.Helix.Assistants) != 1 {
				continue
			}
			if err := tx.Table("org_bots").
				Where("org_id = ? AND id = ? AND agent_app_id IS NULL", candidate.OrgID, candidate.NodeID).
				Update("agent_app_id", candidate.AppID).Error; err != nil {
				return fmt.Errorf("backfill link %s/%s: %w", candidate.OrgID, candidate.NodeID, err)
			}
		}
		return nil
	})
}

func backfillProjectAgentApps(db *gorm.DB) error {
	if !db.Migrator().HasTable("projects") ||
		!db.Migrator().HasTable("org_bot_runtime_state") ||
		!db.Migrator().HasTable("org_bots") ||
		!db.Migrator().HasTable("apps") {
		return nil
	}
	type candidate struct {
		ProjectID       string
		OrganizationID  string
		DefaultAppID    string
		NodeID          string
		BotAppID        string
		RuntimeAgentApp string
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var candidates []candidate
		if err := tx.Raw(`
		SELECT project.id AS project_id,
		       project.organization_id,
		       COALESCE(project.default_helix_app_id, '') AS default_app_id,
		       bot.id AS node_id,
		       COALESCE(bot.agent_app_id, '') AS bot_app_id,
		       COALESCE(agent_state.value, '') AS runtime_agent_app
		FROM projects AS project
		JOIN org_bot_runtime_state AS project_state
		  ON project_state.value = project.id
		 AND project_state.backend = 'helix'
		 AND project_state.key = 'project_id'
		JOIN org_bots AS bot
		  ON bot.org_id = project_state.org_id
		 AND bot.id = project_state.bot_id
		LEFT JOIN org_bot_runtime_state AS agent_state
		  ON agent_state.org_id = project_state.org_id
		 AND agent_state.bot_id = project_state.bot_id
		 AND agent_state.backend = 'helix'
		 AND agent_state.key = 'agent_app_id'
		WHERE project.organization_id = bot.org_id
		  AND project.deleted_at IS NULL
		`).Scan(&candidates).Error; err != nil {
			return fmt.Errorf("list project agent app candidates: %w", err)
		}
		projectApps := make(map[string]map[string]struct{})
		projectBotCounts := make(map[string]int)
		for _, candidate := range candidates {
			projectKey := candidate.OrganizationID + "\x00" + candidate.ProjectID
			projectBotCounts[projectKey]++
			if projectApps[projectKey] == nil {
				projectApps[projectKey] = make(map[string]struct{})
			}
			projectApps[projectKey][candidate.BotAppID] = struct{}{}
		}
		for _, candidate := range candidates {
			projectKey := candidate.OrganizationID + "\x00" + candidate.ProjectID
			if projectBotCounts[projectKey] != 1 || len(projectApps[projectKey]) != 1 {
				continue
			}
			validApp := func(appID string) (bool, error) {
				if appID == "" {
					return false, nil
				}
				var app struct {
					Config string
				}
				if err := tx.Table("apps").Select("CAST(config AS TEXT) AS config").
					Where("id = ? AND organization_id = ?", appID, candidate.OrganizationID).
					Take(&app).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return false, nil
					}
					return false, err
				}
				var config types.AppConfig
				if err := json.Unmarshal([]byte(app.Config), &config); err != nil {
					return false, nil
				}
				if len(config.Helix.Assistants) != 1 {
					return false, nil
				}
				var claims int64
				if err := tx.Table("org_bots").
					Where("org_id = ? AND agent_app_id = ? AND id <> ?", candidate.OrganizationID, appID, candidate.NodeID).
					Count(&claims).Error; err != nil {
					return false, err
				}
				return claims == 0, nil
			}

			canonicalAppID := candidate.DefaultAppID
			defaultValid, err := validApp(canonicalAppID)
			if err != nil {
				return fmt.Errorf("validate project %s default app: %w", candidate.ProjectID, err)
			}
			if !defaultValid {
				botValid, err := validApp(candidate.BotAppID)
				if err != nil {
					return fmt.Errorf("validate bot %s agent app: %w", candidate.NodeID, err)
				}
				if !botValid {
					continue
				}
				canonicalAppID = candidate.BotAppID
			}

			if candidate.DefaultAppID != canonicalAppID {
				if err := tx.Table("projects").
					Where("id = ? AND organization_id = ?", candidate.ProjectID, candidate.OrganizationID).
					Update("default_helix_app_id", canonicalAppID).Error; err != nil {
					return fmt.Errorf("backfill project %s agent app: %w", candidate.ProjectID, err)
				}
			}
			if candidate.BotAppID != canonicalAppID {
				if err := tx.Table("org_bots").
					Where("org_id = ? AND id = ?", candidate.OrganizationID, candidate.NodeID).
					Update("agent_app_id", canonicalAppID).Error; err != nil {
					return fmt.Errorf("backfill bot %s agent app: %w", candidate.NodeID, err)
				}
			}
			if candidate.RuntimeAgentApp != canonicalAppID {
				state := nodeRuntimeStateRow{
					OrgID: candidate.OrganizationID, NodeID: candidate.NodeID,
					Backend: "helix", Key: "agent_app_id", Value: canonicalAppID,
				}
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "org_id"}, {Name: "bot_id"}, {Name: "backend"}, {Name: "key"}},
					DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
				}).Create(&state).Error; err != nil {
					return fmt.Errorf("backfill bot %s runtime agent app: %w", candidate.NodeID, err)
				}
			}
		}
		return nil
	})
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

// The org_topics / org_subscriptions rename entries above still apply:
// the tables are no longer AutoMigrated, but the conversion in
// application/cutover reads them, and an upgrade from a pre-rename
// release must still find its data under the current names.

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
// that make org_reporting_lines self-clean when a Bot is deleted:
// (org_id, manager_id) and (org_id, report_id) both reference
// org_bots(org_id, id). Idempotent — re-adding an existing constraint
// is a no-op. Postgres-specific (our production target); unit tests run
// against the in-memory store and never reach here.
func installReportingLineFKs(db *gorm.DB) error {
	if !db.Migrator().HasTable("org_reporting_lines") || !db.Migrator().HasTable("org_bots") {
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
						REFERENCES org_bots(org_id, id)
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
