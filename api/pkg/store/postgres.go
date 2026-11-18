package store

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // postgres query builder
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres migrations
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq" // enable postgres driver

	"github.com/helixml/helix/api/pkg/config"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/orgstore"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type PostgresStore struct {
	cfg config.Store
	*SecretIntakePersistence

	gdb    *gorm.DB
	pubsub pubsub.PubSub

	// Embedded org/tenant subsystem (Organizations, Teams, Memberships, Roles,
	// Invitations, AccessGrants + RBAC). Its methods are promoted onto
	// PostgresStore, satisfying the store.Store org methods. Single source of
	// truth, shared with downstream services (HelixOS) via the orgstore package.
	*orgstore.Store
}

// GormDB returns the underlying *gorm.DB so adjacent subsystems
// (notably the embedded helix-org org-store, H4.3) can land their
// own tables in the same Postgres database without opening a
// second connection pool. The connection lifecycle stays owned by
// PostgresStore; callers must not call (*sql.DB).Close on it.
func (s *PostgresStore) GormDB() *gorm.DB {
	return s.gdb
}

// withGormDB returns a copy of the store whose queries, including those of the
// embedded org store, run on db. Migrations use it to run on a dedicated
// connection pool with its own session settings (lock_timeout).
func (s *PostgresStore) withGormDB(db *gorm.DB) *PostgresStore {
	c := *s
	c.gdb = db
	c.Store = orgstore.New(db)
	c.SecretIntakePersistence = NewSecretIntakePersistence(db)
	return &c
}

func NewPostgresStore(
	cfg config.Store,
	pubsub pubsub.PubSub,
) (*PostgresStore, error) {

	schema.RegisterSerializer("json", schema.JSONSerializer{})

	// Waiting for connection
	gormDB, err := connect(context.Background(), connectConfig{
		host:            cfg.Host,
		port:            cfg.Port,
		schemaName:      cfg.Schema,
		database:        cfg.Database,
		username:        cfg.Username,
		password:        cfg.Password,
		ssl:             cfg.SSL,
		idleConns:       cfg.IdleConns,
		maxConns:        cfg.MaxConns,
		maxConnIdleTime: cfg.MaxConnIdleTime,
		maxConnLifetime: cfg.MaxConnLifetime,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Postgres: %w", err)
	}

	store := &PostgresStore{
		cfg:                     cfg,
		gdb:                     gormDB,
		pubsub:                  pubsub,
		Store:                   orgstore.New(gormDB),
		SecretIntakePersistence: NewSecretIntakePersistence(gormDB),
	}

	if cfg.AutoMigrate {
		err = store.autoMigrate()
		if err != nil {
			return nil, fmt.Errorf("there was an error doing the automigration: %s", err.Error())
		}
	}

	if cfg.SeedModels {
		err = store.seedModels(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to seed models: %w", err)
		}
	}

	return store, nil
}

func (s *PostgresStore) Close() error {
	sqlDB, err := s.gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

type MigrationScript struct {
	Name   string `gorm:"primaryKey"`
	HasRun bool
}

// skipAutoMigrate reports whether HELIX_SKIP_AUTOMIGRATE=1 applies to schema.
// CI sets it when migrations run in a separate step before parallel tests. It
// only covers the public schema: tests that create their own schemas still need
// migrations.
func skipAutoMigrate(schema string) bool {
	return AutoMigrateSkipped() && (schema == "" || schema == "public")
}

// AutoMigrateSkipped reports whether HELIX_SKIP_AUTOMIGRATE=1: migrations ran
// in a separate step (helix serve --migrate-only) and this process must only
// use the schema, as blue/green colours and CI test steps do.
func AutoMigrateSkipped() bool {
	return os.Getenv("HELIX_SKIP_AUTOMIGRATE") == "1"
}

func (s *PostgresStore) autoMigrate() error {
	if skipAutoMigrate(s.cfg.Schema) {
		log.Debug().Msg("skipping automigrate for public schema (HELIX_SKIP_AUTOMIGRATE=1)")
		return nil
	}

	// If schema is specified, check if it exists and if not - create it
	if s.cfg.Schema != "" {
		err := s.gdb.WithContext(context.Background()).Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", s.cfg.Schema)).Error
		if err != nil {
			return err
		}
	}

	// A dedicated pool whose every connection has lock_timeout set, so no
	// migration statement queues behind a lock for long (migration_steps.go).
	migrationDB, err := connect(context.Background(), connectConfig{
		host:            s.cfg.Host,
		port:            s.cfg.Port,
		schemaName:      s.cfg.Schema,
		database:        s.cfg.Database,
		username:        s.cfg.Username,
		password:        s.cfg.Password,
		ssl:             s.cfg.SSL,
		idleConns:       2,
		maxConns:        4,
		maxConnIdleTime: s.cfg.MaxConnIdleTime,
		maxConnLifetime: s.cfg.MaxConnLifetime,
		lockTimeout:     MigrationLockTimeout,
	})
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	ms := s.withGormDB(migrationDB)
	defer func() {
		if err := ms.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close migration connections")
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), migrationLockWait)
	defer cancel()
	return ms.withMigrationLock(ctx, ms.runMigrations)
}

const (
	// migrationLockPollInterval is how often a process waiting for another
	// one's migration retries the advisory lock.
	migrationLockPollInterval = 500 * time.Millisecond
	// migrationLockWait bounds how long startup waits for another process's
	// migration; past it startup fails instead of hanging on a stuck holder.
	migrationLockWait = 30 * time.Minute
)

// withMigrationLock runs migrate while holding a Postgres advisory lock scoped
// to this database and schema, so API replicas starting together migrate one
// at a time instead of racing on CREATE TABLE / CREATE TYPE (which fails with
// a duplicate key on pg_type_typname_nsp_index).
//
// The lock is polled with pg_try_advisory_lock rather than awaited with
// pg_advisory_lock: a backend blocked inside pg_advisory_lock holds a
// snapshot, and the holder's CREATE INDEX CONCURRENTLY waits for every older
// snapshot to finish, which would deadlock the two processes. The lock is held
// on a dedicated connection and released when it closes, including when the
// process dies mid-migration.
func (s *PostgresStore) withMigrationLock(ctx context.Context, migrate func() error) error {
	sqlDB, err := s.gdb.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("get migration lock connection: %w", err)
	}
	defer conn.Close()

	key := "helix-automigrate:" + s.cfg.Schema
	for logged := false; ; {
		var acquired bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtext($1))", key).Scan(&acquired); err != nil {
			return fmt.Errorf("acquire migration lock: %w", err)
		}
		if acquired {
			break
		}
		if !logged {
			log.Info().Str("schema", s.cfg.Schema).Msg("waiting for another process to finish database migrations")
			logged = true
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("acquire migration lock: %w", ctx.Err())
		case <-time.After(migrationLockPollInterval):
		}
	}

	migrateErr := migrate()
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock(hashtext($1))", key); err != nil && migrateErr == nil {
		return fmt.Errorf("release migration lock: %w", err)
	}
	return migrateErr
}

// runMigrations brings the schema and data up to date. Every step is
// idempotent and, on a database that is already migrated, reads only
// (TestRunMigrationsIsIdempotent): a write takes locks on a live database,
// and the previous release keeps serving while a blue/green deploy migrates.
// Each step retries while it hits lock_timeout (migrationStep).
//
// Schema changes must stay additive so the previous release still works on
// the new schema; see migrations/README.md.
func (s *PostgresStore) runMigrations() error {
	ctx := context.Background()
	m := s.gdb.Migrator()

	// Running migrations from ./migrations directory,
	// ref: https://github.com/golang-migrate/migrate
	if err := s.migrationStep("golang-migrate", s.MigrateUp); err != nil {
		return err
	}

	// Queue mode (interrupt=false) is the safe default — interrupt mode is the
	// exceptional case. GORM AutoMigrate doesn't change column defaults, so set
	// it here, only when the catalog still has the old default.
	if m.HasTable(&types.PromptHistoryEntry{}) {
		if err := s.migrationStep("prompt_history_entries.interrupt default", func() error {
			def, err := s.columnDefault(ctx, "prompt_history_entries", "interrupt")
			if err != nil || def == "false" {
				return err
			}
			return s.gdb.WithContext(ctx).Exec(
				"ALTER TABLE prompt_history_entries ALTER COLUMN interrupt SET DEFAULT false").Error
		}); err != nil {
			return err
		}
	}

	// Truncate oversized session names before AutoMigrate adds the
	// varchar(255) constraint. Input validation in CreateSession/UpdateSession/
	// UpdateSessionName prevents new oversized names.
	if m.HasTable(&types.Session{}) {
		if err := s.migrationStep("sessions.name truncate", func() error {
			return s.backfillRows(ctx, "sessions", "name = LEFT(name, 255)", nil, "LENGTH(name) > 255")
		}); err != nil {
			return err
		}
	}

	if err := s.migrationStep("automigrate", func() error {
		return s.gdb.WithContext(ctx).AutoMigrate(
			&types.Organization{},
			&types.User{},
			&types.Team{},
			&types.TeamMembership{},
			&types.Role{},
			&types.OrganizationMembership{},
			&types.OrganizationInvitation{},
			&types.AccessGrant{},
			&types.AccessGrantRoleBinding{},
			&types.UserMeta{},
			&types.Session{},
			&types.Interaction{},
			&types.App{},
			&types.ApiKey{},
			&types.Tool{},
			&types.Knowledge{},
			&types.KnowledgeVersion{},
			&types.DataEntity{},
			&types.LLMCall{},
			&MigrationScript{},
			&types.Secret{},
			&types.LicenseKey{},
			&types.ProviderEndpoint{},
			&types.OAuthProvider{},
			&types.OAuthConnection{},
			&types.OAuthRequestToken{},
			&types.UserSession{},
			&types.GitProviderConnection{},
			&types.ServiceConnection{},
			&types.UsageMetric{},
			&types.Model{},
			&types.DynamicModelInfo{},
			&types.StepInfo{},
			&types.RunnerProfile{},
			&types.RunnerAssignment{},
			&types.RunnerToken{},
			&types.SlackThread{},
			&types.TeamsThread{},
			&types.CrispThread{},
			&types.TriggerConfiguration{},
			&types.TriggerExecution{},
			&types.SystemSettings{},
			&types.Wallet{},
			&types.Transaction{},
			&types.WebSearchRequest{},
			&types.TopUp{},
			&types.SubscriptionInvoiceCredit{},
			&types.Project{},
			&types.Artifact{},
			&types.ArtifactVersion{},
			&types.SecretIntake{},
			&types.WebhookEndpoint{},
			&types.WebhookEvent{},
			&types.WebhookDelivery{},
			&types.ProjectAuditLog{},   // Audit trail for project activity
			&types.OrgAuditLog{},       // Audit trail for Helix org activity
			&types.RequestAuditEvent{}, // Audit trail for API requests (incident forensics)
			&types.IdentityAttestationRedemption{},
			&types.SampleProject{},
			&types.SpecTask{},
			&types.SpecTaskWorkSession{},
			&types.SpecTaskZedThread{},
			&types.SpecTaskExternalAgent{},
			&types.SpecTaskDesignReview{},
			&types.SpecTaskPRProposal{},
			&types.SpecTaskDesignReviewComment{},
			&types.SpecTaskDesignReviewCommentReply{},
			&types.SpecTaskGitPushEvent{},
			&types.SpecTaskAttachment{},
			&types.GitRepository{},
			&types.GitPullRequest{},
			&types.ProjectRepository{}, // Junction table for project-repository many-to-many relationship
			&types.SpecTaskImplementationTask{},
			&types.AgentRunner{},
			&types.ZedSettingsOverride{},
			&types.Memory{},
			&types.SandboxInstance{},
			&types.Sandbox{},
			&types.OrgCodeAgentHarness{},
			&types.DiskUsageHistory{},
			&types.GuidelinesHistory{},
			&types.PromptHistoryEntry{},
			&types.GlobalCounter{},
			&types.CloneGroup{},
			&types.ClaudeSubscription{},
			&types.CodexSubscription{},
			&types.AttentionEvent{},
			&types.EvaluationSuite{},
			&types.EvaluationRun{},
			&types.VHostRoute{},
			&types.ProjectWebServiceState{},
			&types.WebServiceDeploy{},
			&types.SandboxCacheState{}, // golden_builds
			&types.WebPage{},
		)
	}); err != nil {
		return err
	}
	if err := s.migrationStep("automigrate llm_call_payloads", func() error {
		return s.gdb.Set("gorm:table_options", "PARTITION BY RANGE (created)").AutoMigrate(&llmCallPayload{})
	}); err != nil {
		return err
	}

	// users.token held whatever bearer token a request carried when the row
	// was last saved. User.Token is no longer persisted; clear stored values.
	// Skipped where the column never existed.
	if m.HasColumn("users", "token") {
		if err := s.migrationStep("users.token clear", func() error {
			return s.backfillRows(ctx, "users", "token = ''", nil, "token IS NOT NULL AND token <> ''")
		}); err != nil {
			return err
		}
	}

	// Indexes built in code rather than by AutoMigrate or a migrations/*.sql
	// file: CONCURRENTLY doesn't block writes while it builds, cannot run in a
	// transaction block (so not in golang-migrate, which also runs before
	// AutoMigrate creates the tables on fresh databases), and GORM has no
	// option for it.
	for _, idx := range []struct{ name, ddl string }{
		{"idx_llm_calls_legacy_payload",
			"CREATE INDEX CONCURRENTLY idx_llm_calls_legacy_payload ON llm_calls (created,id) WHERE " + legacyPayload},
		{"idx_llm_calls_request_id",
			"CREATE INDEX CONCURRENTLY idx_llm_calls_request_id ON llm_calls (request_id)"},
		// GetProjectExploratorySession looks sessions up by
		// (config->>'project_id', config->>'session_role'); the org-bot
		// transcript Mirror polls it every 5s per tracked worker, and without
		// this index each call was a parallel seq scan that pinned all 16 cores
		// of the prod runner on 2026-10-04.
		{"idx_sessions_config_project_id_session_role",
			"CREATE INDEX CONCURRENTLY idx_sessions_config_project_id_session_role ON sessions ((config->>'project_id'), (config->>'session_role'))"},
		{"idx_sessions_config_provider_recovery_provider",
			"CREATE INDEX CONCURRENTLY idx_sessions_config_provider_recovery_provider ON sessions ((config->'provider_recovery'->>'provider'))"},
		// One interaction per agent-reported user entry: overlapping readers of
		// a reconnecting agent can both pass the read-side dedupe in
		// userEntryAlreadyRecorded, and this index makes the second insert fail
		// (CreateInteraction reports it as ErrDuplicateExternalAgentUserEntry).
		// The text is part of the key, matching the read-side rule: an index an
		// agent restart renumbered can carry a genuinely new message. Partial,
		// so turns Helix sent (no entry id) are unaffected.
		{externalAgentUserEntryIndex,
			"CREATE UNIQUE INDEX CONCURRENTLY " + externalAgentUserEntryIndex +
				" ON interactions (session_id, generation_id, external_agent_user_thread_id, external_agent_user_entry_id, md5(prompt_message))" +
				" WHERE external_agent_user_entry_id <> ''"},
		// StepInfo and UsageMetric once declared these as GORM indexes named
		// idx_app_interaction / idx_project_spec_task, names llm_calls already
		// used, so they were never built and AutoMigrate re-issued them on
		// every boot.
		{"idx_step_infos_app_interaction",
			"CREATE INDEX CONCURRENTLY idx_step_infos_app_interaction ON step_infos (app_id, interaction_id)"},
		{"idx_usage_metrics_project_spec_task",
			"CREATE INDEX CONCURRENTLY idx_usage_metrics_project_spec_task ON usage_metrics (project_id, spec_task_id)"},
	} {
		if err := s.migrationStep("index "+idx.name, func() error {
			return s.createIndexConcurrently(ctx, idx.name, idx.ddl)
		}); err != nil {
			return err
		}
	}

	for _, b := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{"backfill apps.agent_kind", s.backfillAgentKinds},
		{"backfill users.email_verified", s.backfillEmailVerified},
		{"backfill sessions external_agent_id", s.backfillSessionAgentConnections},
	} {
		if err := s.migrationStep(b.name, func() error { return b.fn(ctx) }); err != nil {
			return err
		}
	}

	// Relax prompt_history_entries.spec_task_id / project_id to nullable. The
	// queue is session-scoped: general/system sends (org bots via
	// POST /sessions/{id}/messages, and any non-spec-task session send)
	// enqueue by session with no spec task. AutoMigrate does not reliably DROP
	// NOT NULL, so do it explicitly, only while a column still has it.
	if m.HasTable(&types.PromptHistoryEntry{}) {
		if err := s.migrationStep("prompt_history_entries nullable ids", func() error {
			for _, col := range []string{"spec_task_id", "project_id"} {
				nullable, err := s.columnNullable(ctx, "prompt_history_entries", col)
				if err != nil {
					return err
				}
				if !nullable {
					if err := s.gdb.WithContext(ctx).Exec(
						"ALTER TABLE prompt_history_entries ALTER COLUMN " + col + " DROP NOT NULL").Error; err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}

	// Backfill secret scope for rows created before the column existed.
	// AutoMigrate adds the column with default 'dev', but pre-existing rows can
	// end up NULL/empty depending on the driver; 'dev' is the default scope and
	// preserves the pre-feature behaviour (project secrets were dev-only).
	if m.HasTable(&types.Secret{}) {
		if err := s.migrationStep("backfill secrets.scope", func() error {
			return s.backfillRows(ctx, "secrets", "scope = 'dev'", nil, "scope IS NULL OR scope = ''")
		}); err != nil {
			return err
		}
	}

	// Link pre-existing session-backed sandbox rows to the helix-org bot whose
	// session owns them. New rows get org_bot_id from the executor at
	// BeginSession.
	if m.HasTable(&types.Sandbox{}) && m.HasTable(&types.Session{}) {
		if err := s.migrationStep("backfill sandboxes.org_bot_id", func() error {
			const unlinked = `FROM sandboxes sb JOIN sessions s ON sb.session_id = s.id
				WHERE COALESCE(sb.org_bot_id, '') = '' AND COALESCE(s.config->>'org_worker_id', '') <> ''`
			found, err := s.exists(ctx, "SELECT 1 "+unlinked)
			if err != nil || !found {
				return err
			}
			return s.execBatches(ctx, fmt.Sprintf(`
				UPDATE sandboxes SET org_bot_id = s.config->>'org_worker_id'
				FROM sessions s
				WHERE sandboxes.session_id = s.id
				  AND sandboxes.id IN (SELECT sb.id %s LIMIT %d)`, unlinked, backfillBatchSize))
		}); err != nil {
			return err
		}
	}

	if err := s.migrationStep("encrypt legacy plaintext secrets", func() error {
		return s.encryptLegacyPlaintextSecrets(ctx, encryptLegacySecretsBatchSize)
	}); err != nil {
		return err
	}

	if err := s.migrationStep("golden builds out of project metadata", func() error {
		return s.migrateGoldenBuildsFromProjectMetadata(ctx)
	}); err != nil {
		return err
	}

	if err := s.migrationStep("role config", func() error {
		return s.AutoMigrateRoleConfig(ctx)
	}); err != nil {
		return err
	}

	if err := s.migrationStep("foreign keys", func() error {
		s.createFKs()
		return nil
	}); err != nil {
		return err
	}

	// Ensure default project exists for spec tasks
	if err := s.migrationStep("default project", func() error {
		return s.ensureDefaultProject(ctx)
	}); err != nil {
		return err
	}

	// helix-org's org_* tables reference organizations, so they migrate last.
	return s.migrationStep("helix-org tables", func() error {
		return orggorm.Migrate(s.gdb, orggorm.Options{InstallOrganizationFK: true})
	})
}

// createFKs installs the foreign keys AutoMigrate doesn't. A failure is
// logged, not returned: the next migration retries it.
func (s *PostgresStore) createFKs() {
	if err := createFK(s.gdb, types.OrganizationMembership{}, types.Organization{}, "organization_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.OrganizationInvitation{}, types.Organization{}, "organization_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.Team{}, types.Organization{}, "organization_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.Role{}, types.Organization{}, "organization_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.TeamMembership{}, types.Team{}, "team_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.TeamMembership{}, types.User{}, "user_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.AccessGrantRoleBinding{}, types.AccessGrant{}, "access_grant_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.AccessGrant{}, types.Organization{}, "organization_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.ApiKey{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.KnowledgeVersion{}, types.Knowledge{}, "knowledge_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.StepInfo{}, types.Session{}, "session_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.Interaction{}, types.Session{}, "session_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.SlackThread{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.CrispThread{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.TriggerConfiguration{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.TriggerExecution{}, types.TriggerConfiguration{}, "trigger_configuration_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.Memory{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.EvaluationRun{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.EvaluationSuite{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	// Project-Repository junction table FKs (cascade delete when project or repo is deleted)
	if err := createFK(s.gdb, types.ProjectRepository{}, types.Project{}, "project_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK for project_repositories -> projects")
	}
	if err := createFK(s.gdb, types.ProjectRepository{}, types.GitRepository{}, "repository_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK for project_repositories -> git_repositories")
	}

	if err := createFK(s.gdb, types.SandboxCacheState{}, types.Project{}, "project_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK for golden_builds -> projects")
	}
}

type namedTable interface {
	TableName() string
}

// createFK creates a foreign key relationship between two tables.
//
// The argument `src` is the table with the field (`fk`) which refers to the field `pk` in the other (`dest`) table.
func createFK(db *gorm.DB, src, dst interface{}, fk, pk string, onDelete, onUpdate string) error {
	var (
		srcTableName string
		dstTableName string
	)

	sourceType := reflect.TypeOf(src)
	_, ok := sourceType.MethodByName("TableName")
	if ok {
		srcTableName = src.(namedTable).TableName()
	} else {
		srcTableName = db.NamingStrategy.TableName(sourceType.Name())
	}

	destinationType := reflect.TypeOf(dst)
	_, ok = destinationType.MethodByName("TableName")
	if ok {
		dstTableName = dst.(namedTable).TableName()
	} else {
		dstTableName = db.NamingStrategy.TableName(destinationType.Name())
	}

	// Dealing with custom table names that contain schema in them
	// Use a hash-based approach to ensure constraint names stay under PostgreSQL's 63-character limit
	// while remaining deterministic and avoiding collisions
	constraintName := generateConstraintName(srcTableName, dstTableName, fk)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if !db.Migrator().HasConstraint(src, constraintName) {
		err := db.WithContext(ctx).Exec(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s(%s) ON DELETE %s ON UPDATE %s",
			srcTableName,
			constraintName,
			fk,
			dstTableName,
			pk,
			onDelete,
			onUpdate)).Error
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
	}
	return nil
}

// generateConstraintName creates a deterministic constraint name that stays under PostgreSQL's 63-character limit
// It uses a hash-based approach to avoid collisions while keeping names readable
func generateConstraintName(srcTableName, dstTableName, fkColumn string) string {
	// Extract just the table names without schema prefixes for readability
	srcTable := extractTableName(srcTableName)
	dstTable := extractTableName(dstTableName)

	// Create a base name that's human-readable
	baseName := fmt.Sprintf("fk_%s_%s_%s", srcTable, dstTable, fkColumn)

	// If the base name is short enough AND the table names don't contain schema prefixes, use it as-is
	if len(baseName) <= 63 && !strings.Contains(srcTableName, ".") && !strings.Contains(dstTableName, ".") {
		return baseName
	}

	// Otherwise, create a hash-based name that includes the full table names for uniqueness
	// This ensures different schemas produce different constraint names
	fullName := fmt.Sprintf("%s_%s_%s", srcTableName, dstTableName, fkColumn)
	hash := sha256.Sum256([]byte(fullName))
	hashStr := hex.EncodeToString(hash[:])[:16] // Use first 16 characters of hash

	// Create a constraint name that's guaranteed to be under 63 characters
	constraintName := fmt.Sprintf("fk_%s_%s_%s", srcTable, dstTable, hashStr)

	// Final safety check - truncate if still too long (shouldn't happen with our format)
	if len(constraintName) > 63 {
		constraintName = constraintName[:63]
	}

	return constraintName
}

// extractTableName extracts the table name from a potentially schema-qualified table name
func extractTableName(fullTableName string) string {
	parts := strings.Split(fullTableName, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1] // Return the last part (table name)
	}
	return fullTableName
}

type Scanner interface {
	Scan(dest ...interface{}) error
}

// Compile-time interface check:
var _ Store = (*PostgresStore)(nil)

func (s *PostgresStore) MigrateUp() error {
	migrations, err := s.GetMigrations()
	if err != nil {
		return err
	}
	defer migrations.Close()

	err = migrations.Up()
	if err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func (s *PostgresStore) MigrateDown() error {
	migrations, err := s.GetMigrations()
	if err != nil {
		return err
	}
	err = migrations.Down()
	if err != migrate.ErrNoChange {
		return err
	}
	return nil
}

//go:embed migrations/*.sql
var fs embed.FS

func (s *PostgresStore) GetMigrations() (*migrate.Migrate, error) {
	files, err := iofs.New(fs, "migrations")
	if err != nil {
		return nil, err
	}

	// Read SSL setting from environment
	sqlSettings := "sslmode=disable"
	if s.cfg.SSL {
		sqlSettings = "sslmode=require"
	}

	if s.cfg.Schema != "" {
		sqlSettings += fmt.Sprintf("&search_path=%s", s.cfg.Schema)
	}
	// lib/pq sends parameters it doesn't know as session settings.
	sqlSettings += fmt.Sprintf("&lock_timeout=%d", MigrationLockTimeout.Milliseconds())

	connectionString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?%s",
		s.cfg.Username,
		s.cfg.Password,
		s.cfg.Host,
		s.cfg.Port,
		s.cfg.Database,
		sqlSettings,
	)

	migrations, err := migrate.NewWithSourceInstance(
		"iofs",
		files,
		fmt.Sprintf("%s&&x-migrations-table=helix_migrations", connectionString),
	)
	if err != nil {
		return nil, err
	}
	return migrations, nil
}

// Available DB types
const (
	DatabaseTypePostgres = "postgres"
)

type connectConfig struct {
	host            string
	port            int
	schemaName      string
	database        string
	username        string
	password        string
	ssl             bool
	idleConns       int
	maxConns        int
	maxConnIdleTime time.Duration
	maxConnLifetime time.Duration
	// lockTimeout, when set, is every connection's lock_timeout.
	lockTimeout time.Duration
}

func connect(ctx context.Context, cfg connectConfig) (*gorm.DB, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("sql store startup deadline exceeded")
		default:
			log.Info().
				Str("host", cfg.host).
				Int("port", cfg.port).
				Str("database", cfg.database).
				Msg("connecting to DB")

			var (
				err       error
				dialector gorm.Dialector
			)

			// Read SSL setting from environment
			sslSettings := "sslmode=disable"
			if cfg.ssl {
				sslSettings = "sslmode=require"
			}

			dsn := fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s %s",
				cfg.username, cfg.password, cfg.host, cfg.port, cfg.database, sslSettings)

			// When using a custom schema, set search_path so Postgres resolves
			// unqualified table names within that schema. Without this, GORM's
			// NamingStrategy{TablePrefix} can leak FK constraints into the public
			// schema because the connection's search_path defaults to public.
			var options []string
			if cfg.schemaName != "" {
				options = append(options, "-csearch_path="+cfg.schemaName)
			}
			if cfg.lockTimeout > 0 {
				options = append(options, fmt.Sprintf("-clock_timeout=%d", cfg.lockTimeout.Milliseconds()))
			}
			if len(options) > 0 {
				dsn += fmt.Sprintf(" options='%s'", strings.Join(options, " "))
			}

			dialector = gormpostgres.Open(dsn)

			gormConfig := &gorm.Config{
				Logger: NewGormLogger(time.Second, true),
			}

			if cfg.schemaName != "" {
				gormConfig.NamingStrategy = schema.NamingStrategy{
					TablePrefix: cfg.schemaName + ".",
				}
			}

			db, err := gorm.Open(dialector, gormConfig)
			if err != nil {
				time.Sleep(1 * time.Second)

				log.Err(err).Msg("sql store connector can't reach DB, waiting")

				continue
			}

			sqlDB, err := db.DB()
			if err != nil {
				return nil, err
			}
			sqlDB.SetMaxIdleConns(cfg.idleConns)
			sqlDB.SetMaxOpenConns(cfg.maxConns)
			sqlDB.SetConnMaxIdleTime(cfg.maxConnIdleTime)
			sqlDB.SetConnMaxLifetime(cfg.maxConnLifetime)

			log.Info().Msg("sql store connected")

			// success
			return db, nil
		}
	}
}

func (s *PostgresStore) GetAppCount() (int, error) {
	var count int
	err := s.gdb.Raw("SELECT COUNT(*) FROM apps").Scan(&count).Error
	if err != nil {
		return 0, fmt.Errorf("error getting app count: %w", err)
	}
	return count, nil
}

// IncrementProjectTaskNumber atomically increments NextTaskNumber and returns the current value
// Uses raw SQL with RETURNING for database-level atomicity - no race conditions
// Returns the value BEFORE incrementing (so first task gets 1, second gets 2, etc.)
func (s *PostgresStore) IncrementProjectTaskNumber(ctx context.Context, projectID string) (int, error) {
	var currentNumber int
	err := s.gdb.WithContext(ctx).Raw(`
		UPDATE projects
		SET next_task_number = next_task_number + 1
		WHERE id = ?
		RETURNING next_task_number - 1
	`, projectID).Scan(&currentNumber).Error
	if err != nil {
		return 0, fmt.Errorf("error incrementing project task number: %w", err)
	}
	return currentNumber, nil
}

// IncrementGlobalTaskNumber atomically increments the global task counter and returns the new value
// Uses INSERT ON CONFLICT (upsert) to handle first-time creation, then UPDATE RETURNING for atomicity
// Returns the value AFTER incrementing (so first task gets 1, second gets 2, etc.)
func (s *PostgresStore) IncrementGlobalTaskNumber(ctx context.Context) (int, error) {
	var newNumber int
	err := s.gdb.WithContext(ctx).Raw(`
		INSERT INTO global_counters (name, value, updated_at)
		VALUES ('task_number', 1, NOW())
		ON CONFLICT (name) DO UPDATE
		SET value = global_counters.value + 1, updated_at = NOW()
		RETURNING value
	`).Scan(&newNumber).Error
	if err != nil {
		return 0, fmt.Errorf("error incrementing global task number: %w", err)
	}
	return newNumber, nil
}

// SetProjectPrimaryRepository sets the primary repository for a project
func (s *PostgresStore) SetProjectPrimaryRepository(ctx context.Context, projectID string, repoID string) error {
	err := s.gdb.WithContext(ctx).Model(&types.Project{}).Where("id = ?", projectID).Update("default_repo_id", repoID).Error
	if err != nil {
		return fmt.Errorf("error setting project primary repository: %w", err)
	}
	return nil
}

// AttachRepositoryToProject attaches a repository to a project via the junction table.
func (s *PostgresStore) AttachRepositoryToProject(ctx context.Context, projectID string, repoID string) error {
	if projectID == "" || repoID == "" {
		return fmt.Errorf("project id or repository id not specified")
	}

	// Get repository to find organization_id
	repo, err := s.GetGitRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	// Write to junction table (idempotent - does nothing if already exists)
	err = s.CreateProjectRepository(ctx, projectID, repoID, repo.OrganizationID)
	if err != nil {
		return fmt.Errorf("error creating project repository junction: %w", err)
	}

	return nil
}

// DetachRepositoryFromProject detaches a repository from a specific project.
// NOTE: Signature changed to require projectID since a repo can now be attached to multiple projects.
func (s *PostgresStore) DetachRepositoryFromProject(ctx context.Context, projectID string, repoID string) error {
	if projectID == "" {
		return fmt.Errorf("project id not specified")
	}
	if repoID == "" {
		return fmt.Errorf("repository id not specified")
	}

	// Delete from junction table
	err := s.DeleteProjectRepository(ctx, projectID, repoID)
	if err != nil {
		return fmt.Errorf("error deleting project repository junction: %w", err)
	}

	return nil
}

// CreateSampleProject creates a new sample project
func (s *PostgresStore) CreateSampleProject(ctx context.Context, sample *types.SampleProject) (*types.SampleProject, error) {
	err := s.gdb.WithContext(ctx).Create(sample).Error
	if err != nil {
		return nil, fmt.Errorf("error creating sample project: %w", err)
	}
	return sample, nil
}

// GetSampleProject gets a sample project by ID
func (s *PostgresStore) GetSampleProject(ctx context.Context, id string) (*types.SampleProject, error) {
	var sample types.SampleProject
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&sample).Error
	if err != nil {
		return nil, fmt.Errorf("error getting sample project: %w", err)
	}
	return &sample, nil
}

// ListSampleProjects lists all available sample projects
func (s *PostgresStore) ListSampleProjects(ctx context.Context) ([]*types.SampleProject, error) {
	var samples []*types.SampleProject
	err := s.gdb.WithContext(ctx).Order("created_at DESC").Find(&samples).Error
	if err != nil {
		return nil, fmt.Errorf("error listing sample projects: %w", err)
	}
	return samples, nil
}

// DeleteSampleProject deletes a sample project by ID
func (s *PostgresStore) DeleteSampleProject(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.SampleProject{}, "id = ?", id).Error
	if err != nil {
		return fmt.Errorf("error deleting sample project: %w", err)
	}
	return nil
}

// ensureDefaultProject ensures that the default project exists for spec tasks
// This project is used as a singleton board for all spec tasks until multi-project support is fully implemented
func (s *PostgresStore) ensureDefaultProject(ctx context.Context) error {
	var project types.Project
	err := s.gdb.WithContext(ctx).Where("id = ?", "default").First(&project).Error

	if err == gorm.ErrRecordNotFound {
		// Create default project with board settings
		defaultProject := &types.Project{
			ID:          "default",
			Name:        "Default Project",
			Description: "Default project for spec-driven tasks",
			Status:      "active",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			Metadata: types.ProjectMetadata{
				BoardSettings: &types.BoardSettings{
					WIPLimits: types.WIPLimits{
						Planning: 3,
						Review:   2,
					},
				},
			},
		}

		err = s.gdb.WithContext(ctx).Create(defaultProject).Error
		if err != nil {
			return fmt.Errorf("failed to create default project: %w", err)
		}

		log.Info().Msg("Created default project for spec tasks")
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check for default project: %w", err)
	}

	// Project exists, nothing to do
	return nil
}

// GetDB returns the underlying GORM database connection for testing purposes
// This should only be used in tests when direct database access is needed
func (s *PostgresStore) GetDB() *gorm.DB {
	return s.gdb
}
