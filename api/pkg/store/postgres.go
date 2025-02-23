package store

import (
	"context"
	"embed"
	"fmt"
	reflect "reflect"
	"strings"
	"time"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"        // postgres query builder
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres migrations
	_ "github.com/lib/pq"                                      // enable postgres driver

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type PostgresStore struct {
	cfg config.Store

	gdb *gorm.DB
}

func NewPostgresStore(
	cfg config.Store,
) (*PostgresStore, error) {

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

	return store, nil
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
		&types.Membership{},
		&types.Role{},
		&types.OrganizationMembership{},
		&types.AccessGrant{},
		&types.AccessGrantRoleBinding{},
		&types.UserMeta{},
		&types.Session{},
		&types.App{},
		&types.ApiKey{},
		&types.Tool{},
		&types.Knowledge{},
		&types.KnowledgeVersion{},
		&types.DataEntity{},
		&types.ScriptRun{},
		&types.LLMCall{},
		&MigrationScript{},
		&types.Secret{},
		&types.LicenseKey{},
		&types.ProviderEndpoint{},
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

	if err := createFK(s.gdb, types.Membership{}, types.Team{}, "team_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.Membership{}, types.User{}, "user_id", "id", "CASCADE", "CASCADE"); err != nil {
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

	if err := createFK(s.gdb, types.ScriptRun{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.KnowledgeVersion{}, types.Knowledge{}, "knowledge_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
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
	constraintName := "fk_" + strings.ReplaceAll(srcTableName, ".", "_") + "_" + strings.ReplaceAll(dstTableName, ".", "_")

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

			dialector = postgres.Open(dsn)

			gormConfig := &gorm.Config{}

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
