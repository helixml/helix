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
	cfg              config.Store
	connectionString string

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
	err := s.gdb.WithContext(context.Background()).AutoMigrate(
		&types.UserMeta{},
		&types.Session{},
		&types.App{},
		&types.APIKey{},
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

	if err := createFK(s.gdb, types.APIKey{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.ScriptRun{}, types.App{}, "app_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	if err := createFK(s.gdb, types.KnowledgeVersion{}, types.Knowledge{}, "knowledge_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
	}

	return s.runMigrationScripts(MigrationScripts)
}

// loop over each migration script and run it only if it's not already been run
func (s *PostgresStore) runMigrationScripts(migrationScripts map[string]func(*gorm.DB) error) error {
	for name, script := range migrationScripts {
		var ms MigrationScript
		result := s.gdb.First(&ms, "name = ?", name)
		if result.Error == gorm.ErrRecordNotFound {
			if err := script(s.gdb); err != nil {
				return err
			}
			ms.Name = name
			ms.HasRun = true
			if err := s.gdb.Create(&ms).Error; err != nil {
				return err
			}
			log.Printf("Migration script '%s' executed and logged.", name)
		} else if result.Error != nil {
			return result.Error
		} else {
			log.Printf("Migration script '%s' already executed.", name)
		}
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
	migrations, err := migrate.NewWithSourceInstance(
		"iofs",
		files,
		fmt.Sprintf("%s&&x-migrations-table=helix_schema_migrations", s.connectionString),
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

			log.Info().Msg("sql store connecting to DB")

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
			sqlDB.SetMaxIdleConns(50)
			sqlDB.SetMaxOpenConns(25) // TODO: maybe subtract what pool uses
			sqlDB.SetConnMaxIdleTime(time.Hour)
			sqlDB.SetConnMaxLifetime(time.Minute)

			log.Info().Msg("sql store connected")

			// success
			return db, nil
		}
	}
}
