package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	reflect "reflect"
	"strings"
	"time"

	"database/sql"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"        // postgres query builder
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // postgres migrations
	_ "github.com/lib/pq"                                      // enable postgres driver

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/doug-martin/goqu/v9"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type PostgresStore struct {
	cfg              config.Store
	connectionString string
	pgDb             *sql.DB
	db               *goqu.Database

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

	// Read SSL setting from environment
	sslSettings := "sslmode=disable"
	if cfg.SSL {
		sslSettings = "sslmode=require"
	}

	connectionString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?%s",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		sslSettings,
	)
	pgDb, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	dialect := goqu.Dialect("postgres")
	db := dialect.DB(pgDb)

	store := &PostgresStore{
		connectionString: connectionString,
		cfg:              cfg,
		pgDb:             pgDb,
		db:               db,
		gdb:              gormDB,
	}

	if cfg.AutoMigrate {
		err = store.MigrateUp()
		if err != nil {
			return nil, fmt.Errorf("there was an error doing the migration: %s", err.Error())
		}

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
		&types.App{},
		&types.APIKey{},
		&types.Tool{},
		&types.Knowledge{},
		&types.KnowledgeVersion{},
		&types.SessionToolBinding{},
		&types.DataEntity{},
		&types.ScriptRun{},
		&types.LLMCall{},
		&MigrationScript{},
		&types.Secret{},
	)
	if err != nil {
		return err
	}

	if err := createFK(s.gdb, types.SessionToolBinding{}, types.Tool{}, "tool_id", "id", "CASCADE", "CASCADE"); err != nil {
		log.Err(err).Msg("failed to add DB FK")
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

// given an array of field names - return the indexes as a string
// e.g. $1, $2, $3, $4
func getValueIndexes(fields []string) string {
	parts := []string{}
	for i := range fields {
		parts = append(parts, fmt.Sprintf("$%d", i+1))
	}
	return strings.Join(parts, ", ")
}

// given an array of field names - return the indexes as an update
// start at the given offset
// e.g. id = $2, name = $3
func getKeyValueIndexes(fields []string, offset int) string {
	parts := []string{}
	for i, field := range fields {
		parts = append(parts, fmt.Sprintf("%s = $%d", field, i+offset+1))
	}
	return strings.Join(parts, ", ")
}

var (
	UsermetaFields = []string{
		"id",
		"config",
	}

	UsermetaFieldsString = strings.Join(UsermetaFields, ", ")
)

func scanUserMetaRow(row Scanner) (*types.UserMeta, error) {
	user := &types.UserMeta{}
	var config []byte
	err := row.Scan(
		&user.ID,
		&config,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, err
	}
	err = json.Unmarshal(config, &user.Config)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func getUserMetaValues(user *types.UserMeta) ([]interface{}, error) {
	config, err := json.Marshal(user.Config)
	if err != nil {
		return nil, err
	}
	return []interface{}{
		user.ID,
		config,
	}, nil
}

func (s *PostgresStore) GetUserMeta(
	ctx context.Context,
	userID string,
) (*types.UserMeta, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	row := s.pgDb.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT %s
		FROM usermeta WHERE id = $1
	`, UsermetaFieldsString), userID)

	return scanUserMetaRow(row)
}

func (s *PostgresStore) CreateUserMeta(
	ctx context.Context,
	user types.UserMeta,
) (*types.UserMeta, error) {
	values, err := getUserMetaValues(&user)
	if err != nil {
		return nil, err
	}
	_, err = s.pgDb.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO usermeta (
			%s
		) VALUES (
			%s
		)
	`, UsermetaFieldsString, getValueIndexes(UsermetaFields)), values...)

	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *PostgresStore) UpdateUserMeta(
	ctx context.Context,
	user types.UserMeta,
) (*types.UserMeta, error) {
	values, err := getUserMetaValues(&user)
	if err != nil {
		return nil, err
	}
	// prepend the ID to the values
	values = append([]interface{}{user.ID}, values...)

	_, err = s.pgDb.ExecContext(ctx, fmt.Sprintf(`
		UPDATE usermeta SET
			%s
		WHERE id = $1
	`, getKeyValueIndexes(UsermetaFields, 1)), values...)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (s *PostgresStore) EnsureUserMeta(
	ctx context.Context,
	user types.UserMeta,
) (*types.UserMeta, error) {
	existing, err := s.GetUserMeta(ctx, user.ID)
	if err != nil || existing == nil {
		return s.CreateUserMeta(ctx, user)
	}
	return s.UpdateUserMeta(ctx, user)
}

func (s *PostgresStore) UpdateSessionMeta(
	ctx context.Context,
	data types.SessionMetaUpdate,
) (*types.Session, error) {
	if data.Owner != "" {
		_, err := s.pgDb.Exec(`
		UPDATE session SET
			name = $2,
			owner = $3,
			owner_type = $4
		WHERE id = $1
	`, data.ID, data.Name, data.Owner, data.OwnerType)
		if err != nil {
			return nil, err
		}
	} else {
		_, err := s.pgDb.Exec(`
		UPDATE session SET
			name = $2
		WHERE id = $1
	`, data.ID, data.Name)
		if err != nil {
			return nil, err
		}
	}

	return s.GetSession(ctx, data.ID)
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
	// EnvPostgresSSL       = "HELIX_POSTGRES_SSL"
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

			db, err := gorm.Open(dialector, &gorm.Config{})
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
