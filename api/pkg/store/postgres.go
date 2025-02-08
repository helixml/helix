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
	gormDB, err := connect(context.Background(), cfg)
	if err != nil {
		return nil, err
	}

	store := &PostgresStore{
		cfg: cfg,
		gdb: gormDB,
	}

	if cfg.PGVectorEnabled {
		err = store.autoMigratePGVector()
		if err != nil {
			return nil, err
		}
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
	)
	if err != nil {
		return err
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

func (s *PostgresStore) autoMigratePGVector() error {
	err := s.gdb.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error
	if err != nil {
		return fmt.Errorf("failed to create vector extension: %w. Install it manually or disable PGVector RAG (RAG_PGVECTOR_ENABLED env variable)", err)
	}

	err = s.gdb.WithContext(context.Background()).AutoMigrate(
		&types.KnowledgeEmbeddingItem{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto migrate PGVector table: %w", err)
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

func connect(ctx context.Context, cfg config.Store) (*gorm.DB, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("sql store startup deadline exceeded")
		default:
			log.Info().
				Str("host", cfg.Host).
				Int("port", cfg.Port).
				Str("database", cfg.Database).
				Msg("connecting to DB")

			var (
				err       error
				dialector gorm.Dialector
			)

			// Read SSL setting from environment
			sslSettings := "sslmode=disable"
			if cfg.SSL {
				sslSettings = "sslmode=require"
			}

			dsn := fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s %s",
				cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, sslSettings)

			dialector = postgres.Open(dsn)

			gormConfig := &gorm.Config{}

			if cfg.Schema != "" {
				gormConfig.NamingStrategy = schema.NamingStrategy{
					TablePrefix: cfg.Schema + ".",
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
			sqlDB.SetMaxIdleConns(cfg.IdleConns)
			sqlDB.SetMaxOpenConns(cfg.MaxConns)
			sqlDB.SetConnMaxIdleTime(cfg.MaxConnIdleTime)
			sqlDB.SetConnMaxLifetime(cfg.MaxConnLifetime)

			log.Info().Msg("sql store connected")

			// success
			return db, nil
		}
	}
}
