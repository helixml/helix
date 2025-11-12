package store

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"time"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres" // postgres query builder
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres migrations
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq" // enable postgres driver

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type PostgresStore struct {
	cfg config.Store

	gdb *gorm.DB
}

func NewPostgresStore(
	cfg config.Store,
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
		cfg: cfg,
		gdb: gormDB,
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

func (s *PostgresStore) autoMigrate() error {
	// If schema is specified, check if it exists and if not - create it
	if s.cfg.Schema != "" {
		err := s.gdb.WithContext(context.Background()).Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", s.cfg.Schema)).Error
		if err != nil {
			return err
		}
	}

	// Running migrations from ./migrations directory,
	// ref: https://github.com/golang-migrate/migrate
	err := s.MigrateUp()
	if err != nil {
		log.Err(err).Msg("there was an error doing the automigration, some functionality may not work")
		return fmt.Errorf("failed to run version migrations: %w", err)
	}

	err = s.gdb.WithContext(context.Background()).AutoMigrate(
		&types.Organization{},
		&types.User{},
		&types.Team{},
		&types.TeamMembership{},
		&types.Role{},
		&types.OrganizationMembership{},
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
		&types.UsageMetric{},
		&types.Model{},
		&types.DynamicModelInfo{},
		&types.StepInfo{},
		&types.RunnerSlot{},
		&types.SlackThread{},
		&types.CrispThread{},
		&types.TriggerConfiguration{},
		&types.TriggerExecution{},
		&types.SystemSettings{},
		&types.Wallet{},
		&types.Transaction{},
		&types.TopUp{},
		&types.Project{},
		&types.SampleProject{},
		&types.SpecTask{},
		&types.SpecTaskWorkSession{},
		&types.SpecTaskZedThread{},
		&types.SpecTaskExternalAgent{},
		&types.ExternalAgentActivity{},
		&types.SpecTaskDesignReview{},
		&types.SpecTaskDesignReviewComment{},
		&types.SpecTaskDesignReviewCommentReply{},
		&types.SpecTaskGitPushEvent{},
		&types.AgentWorkItem{},
		&types.AgentSession{},
		&types.AgentSessionStatus{},
		&types.HelpRequest{},
		&types.JobCompletion{},
		&GitRepository{},
		&types.SpecTaskImplementationTask{},
		&types.AgentRunner{},
		&types.PersonalDevEnvironment{}, // DEPRECATED - stub for backward compatibility
		&types.SSHKey{},
		&types.ZedSettingsOverride{},
		&types.Memory{},
		&types.QuestionSet{},
		&types.QuestionSetExecution{},
	)
	if err != nil {
		return err
	}

	err = s.autoMigrateRoleConfig(context.Background())
	if err != nil {
		return err
	}

	if err := createFK(s.gdb, types.OrganizationMembership{}, types.Organization{}, "organization_id", "id", "CASCADE", "CASCADE"); err != nil {
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

	if err := createFK(s.gdb, types.QuestionSetExecution{}, types.QuestionSet{}, "question_set_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	// Ensure default project exists for spec tasks
	if err := s.ensureDefaultProject(context.Background()); err != nil {
		log.Err(err).Msg("failed to ensure default project exists")
		return err
	}

	return nil
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

// CreateProject creates a new project
func (s *PostgresStore) CreateProject(ctx context.Context, project *types.Project) (*types.Project, error) {
	err := s.gdb.WithContext(ctx).Create(project).Error
	if err != nil {
		return nil, fmt.Errorf("error creating project: %w", err)
	}
	return project, nil
}

// GetProject gets a project by ID
func (s *PostgresStore) GetProject(ctx context.Context, projectID string) (*types.Project, error) {
	var project types.Project
	err := s.gdb.WithContext(ctx).Where("id = ?", projectID).First(&project).Error
	if err != nil {
		return nil, fmt.Errorf("error getting project: %w", err)
	}
	return &project, nil
}

// UpdateProject updates an existing project
func (s *PostgresStore) UpdateProject(ctx context.Context, project *types.Project) error {
	err := s.gdb.WithContext(ctx).Save(project).Error
	if err != nil {
		return fmt.Errorf("error updating project: %w", err)
	}
	return nil
}

// ListProjects lists all projects for a given user
func (s *PostgresStore) ListProjects(ctx context.Context, userID string) ([]*types.Project, error) {
	var projects []*types.Project
	err := s.gdb.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error
	if err != nil {
		return nil, fmt.Errorf("error listing projects: %w", err)
	}
	return projects, nil
}

// DeleteProject deletes a project by ID
func (s *PostgresStore) DeleteProject(ctx context.Context, projectID string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.Project{}, "id = ?", projectID).Error
	if err != nil {
		return fmt.Errorf("error deleting project: %w", err)
	}
	return nil
}

// GetProjectRepositories gets all repositories attached to a project
func (s *PostgresStore) GetProjectRepositories(ctx context.Context, projectID string) ([]*GitRepository, error) {
	var repos []*GitRepository
	err := s.gdb.WithContext(ctx).Where("project_id = ?", projectID).Find(&repos).Error
	if err != nil {
		return nil, fmt.Errorf("error getting project repositories: %w", err)
	}
	return repos, nil
}

// SetProjectPrimaryRepository sets the primary repository for a project
func (s *PostgresStore) SetProjectPrimaryRepository(ctx context.Context, projectID string, repoID string) error {
	err := s.gdb.WithContext(ctx).Model(&types.Project{}).Where("id = ?", projectID).Update("default_repo_id", repoID).Error
	if err != nil {
		return fmt.Errorf("error setting project primary repository: %w", err)
	}
	return nil
}

// AttachRepositoryToProject attaches a repository to a project
func (s *PostgresStore) AttachRepositoryToProject(ctx context.Context, projectID string, repoID string) error {
	err := s.gdb.WithContext(ctx).Model(&GitRepository{}).Where("id = ?", repoID).Update("project_id", projectID).Error
	if err != nil {
		return fmt.Errorf("error attaching repository to project: %w", err)
	}
	return nil
}

// DetachRepositoryFromProject detaches a repository from its project
func (s *PostgresStore) DetachRepositoryFromProject(ctx context.Context, repoID string) error {
	err := s.gdb.WithContext(ctx).Model(&GitRepository{}).Where("id = ?", repoID).Update("project_id", "").Error
	if err != nil {
		return fmt.Errorf("error detaching repository from project: %w", err)
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
			ID:             "default",
			Name:           "Default Project",
			Description:    "Default project for spec-driven tasks",
			Status:         "active",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			Metadata: datatypes.JSON([]byte(`{
				"board_settings": {
					"wip_limits": {
						"planning": 3,
						"review": 2,
						"implementation": 5
					}
				}
			}`)),
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
