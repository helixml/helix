package store

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	reflect "reflect"
	"strings"
	"time"

	"database/sql"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/doug-martin/goqu/v9"
	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
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

	connectionString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Username,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
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

func (s *PostgresStore) autoMigrate() error {
	err := s.gdb.WithContext(context.Background()).AutoMigrate(
		&types.Tool{},
		&types.SessionToolBinding{},
	)
	if err != nil {
		return err
	}

	if err := createFK(s.gdb, types.SessionToolBinding{}, types.Tool{}, "tool_id", "id", "CASCADE", "CASCADE"); err != nil {
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

// given an array of field names - return the indexes as a string
// e.g. $1, $2, $3, $4
func getValueIndexes(fields []string) string {
	parts := []string{}
	for i := range fields {
		parts = append(parts, fmt.Sprintf("$%d", i+1))
	}
	return fmt.Sprintf("%s", strings.Join(parts, ", "))
}

// given an array of field names - return the indexes as an update
// start at the given offset
// e.g. id = $2, name = $3
func getKeyValueIndexes(fields []string, offset int) string {
	parts := []string{}
	for i, field := range fields {
		parts = append(parts, fmt.Sprintf("%s = $%d", field, i+offset+1))
	}
	return fmt.Sprintf("%s", strings.Join(parts, ", "))
}

var BOT_FIELDS = []string{
	"id",
	"name",
	"created",
	"updated",
	"owner",
	"owner_type",
	"config",
}

var BOT_FIELDS_STRING = strings.Join(BOT_FIELDS, ", ")

func scanBotRow(row Scanner) (*types.Bot, error) {
	bot := &types.Bot{}
	var config []byte
	err := row.Scan(
		&bot.ID,
		&bot.Name,
		&bot.Created,
		&bot.Updated,
		&bot.Owner,
		&bot.OwnerType,
		&config,
	)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(config, &bot.Config)
	if err != nil {
		return nil, err
	}
	return bot, nil
}

func getBotValues(bot *types.Bot) ([]interface{}, error) {
	config, err := json.Marshal(bot.Config)
	if err != nil {
		return nil, err
	}
	return []interface{}{
		bot.ID,
		bot.Name,
		bot.Created,
		bot.Updated,
		bot.Owner,
		bot.OwnerType,
		config,
	}, nil
}

var USERMETA_FIELDS = []string{
	"id",
	"config",
}

var USERMETA_FIELDS_STRING = strings.Join(USERMETA_FIELDS, ", ")

func scanUserMetaRow(row Scanner) (*types.UserMeta, error) {
	user := &types.UserMeta{}
	var config []byte
	err := row.Scan(
		&user.ID,
		&config,
	)
	if err != nil {
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

func (d *PostgresStore) GetBot(
	ctx context.Context,
	botID string,
) (*types.Bot, error) {
	if botID == "" {
		return nil, fmt.Errorf("botID cannot be empty")
	}
	row := d.pgDb.QueryRow(fmt.Sprintf(`
		SELECT %s
		FROM bot WHERE id = $1
	`, BOT_FIELDS_STRING), botID)

	return scanBotRow(row)
}

func (d *PostgresStore) GetUserMeta(
	ctx context.Context,
	userID string,
) (*types.UserMeta, error) {
	if userID == "" {
		return nil, fmt.Errorf("userID cannot be empty")
	}
	row := d.pgDb.QueryRow(fmt.Sprintf(`
		SELECT %s
		FROM usermeta WHERE id = $1
	`, USERMETA_FIELDS_STRING), userID)

	return scanUserMetaRow(row)
}

func (d *PostgresStore) getSessionsWhere(query GetSessionsQuery) goqu.Ex {
	where := goqu.Ex{}
	if query.Owner != "" {
		where["owner"] = query.Owner
	}
	if query.OwnerType != "" {
		where["owner_type"] = query.OwnerType
	}
	return where
}

func (d *PostgresStore) GetBots(
	ctx context.Context,
	query GetBotsQuery,
) ([]*types.Bot, error) {
	where := goqu.Ex{}
	if query.Owner != "" {
		where["owner"] = query.Owner
	}
	if query.OwnerType != "" {
		where["owner_type"] = query.OwnerType
	}

	sqlQuery := d.db.
		From("bot").
		Where(where).
		Order(goqu.I("created").Desc())

	sql, values, err := sqlQuery.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := d.pgDb.Query(sql, values...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bots := []*types.Bot{}
	for rows.Next() {
		bot, err := scanBotRow(rows)
		if err != nil {
			return nil, err
		}
		bots = append(bots, bot)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return bots, nil
}

func (d *PostgresStore) CreateBot(
	ctx context.Context,
	bot types.Bot,
) (*types.Bot, error) {
	values, err := getBotValues(&bot)
	if err != nil {
		return nil, err
	}
	_, err = d.pgDb.Exec(fmt.Sprintf(`
		INSERT INTO bot (
			%s
		) VALUES (
			%s
		)
	`, BOT_FIELDS_STRING, getValueIndexes(BOT_FIELDS)), values...)

	if err != nil {
		return nil, err
	}
	return &bot, nil
}

func (d *PostgresStore) CreateUserMeta(
	ctx context.Context,
	user types.UserMeta,
) (*types.UserMeta, error) {
	values, err := getUserMetaValues(&user)
	if err != nil {
		return nil, err
	}
	_, err = d.pgDb.Exec(fmt.Sprintf(`
		INSERT INTO usermeta (
			%s
		) VALUES (
			%s
		)
	`, USERMETA_FIELDS_STRING, getValueIndexes(USERMETA_FIELDS)), values...)

	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (d *PostgresStore) UpdateBot(
	ctx context.Context,
	bot types.Bot,
) (*types.Bot, error) {
	values, err := getBotValues(&bot)
	if err != nil {
		return nil, err
	}

	// prepend the ID to the values
	values = append([]interface{}{bot.ID}, values...)

	_, err = d.pgDb.Exec(fmt.Sprintf(`
		UPDATE bot SET
			%s
		WHERE id = $1
	`, getKeyValueIndexes(BOT_FIELDS, 1)), values...)

	if err != nil {
		return nil, err
	}

	return &bot, nil
}

func (d *PostgresStore) UpdateUserMeta(
	ctx context.Context,
	user types.UserMeta,
) (*types.UserMeta, error) {
	values, err := getUserMetaValues(&user)
	if err != nil {
		return nil, err
	}
	// prepend the ID to the values
	values = append([]interface{}{user.ID}, values...)

	_, err = d.pgDb.Exec(fmt.Sprintf(`
		UPDATE usermeta SET
			%s
		WHERE id = $1
	`, getKeyValueIndexes(USERMETA_FIELDS, 1)), values...)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func (d *PostgresStore) EnsureUserMeta(
	ctx context.Context,
	user types.UserMeta,
) (*types.UserMeta, error) {
	existing, err := d.GetUserMeta(ctx, user.ID)
	if err != nil || existing == nil {
		return d.CreateUserMeta(ctx, user)
	} else {
		return d.UpdateUserMeta(ctx, user)
	}
}

func (d *PostgresStore) DeleteBot(
	ctx context.Context,
	botID string,
) (*types.Bot, error) {
	deleted, err := d.GetBot(ctx, botID)
	if err != nil {
		return nil, err
	}
	_, err = d.pgDb.Exec(`
		DELETE FROM bot WHERE id = $1
	`, botID)
	if err != nil {
		return nil, err
	}

	return deleted, nil
}

func (d *PostgresStore) UpdateSessionMeta(
	ctx context.Context,
	data types.SessionMetaUpdate,
) (*types.Session, error) {
	if data.Owner != "" {
		_, err := d.pgDb.Exec(`
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
		_, err := d.pgDb.Exec(`
		UPDATE session SET
			name = $2
		WHERE id = $1
	`, data.ID, data.Name)
		if err != nil {
			return nil, err
		}
	}

	return d.GetSession(ctx, data.ID)
}

func (d *PostgresStore) CreateAPIKey(ctx context.Context, owner OwnerQuery, name string) (string, error) {
	// Generate a new API key
	key, err := generateAPIKey()
	if err != nil {
		return "", err
	}

	// Insert the new API key into the database
	sqlStatement := `
insert into api_key (owner, owner_type, key, name)
values ($1, $2, $3, $4)
returning key
`
	var id string
	err = d.pgDb.QueryRow(
		sqlStatement,
		owner.Owner,
		owner.OwnerType,
		key,
		name,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	return id, nil
}

func generateAPIKey() (string, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	return types.API_KEY_PREIX + base64.URLEncoding.EncodeToString(key), nil
}

func (d *PostgresStore) GetAPIKeys(ctx context.Context, query OwnerQuery) ([]*types.ApiKey, error) {
	var apiKeys []*types.ApiKey
	sqlStatement := `
select
	key,
	owner,
	owner_type	
from
	api_key
where
	owner = $1 and owner_type = $2
`
	rows, err := d.pgDb.Query(
		sqlStatement,
		query.Owner,
		query.OwnerType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var apiKey types.ApiKey
		err := rows.Scan(
			&apiKey.Key,
			&apiKey.Owner,
			&apiKey.OwnerType,
		)
		if err != nil {
			return nil, err
		}
		apiKeys = append(apiKeys, &apiKey)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (d *PostgresStore) DeleteAPIKey(ctx context.Context, apiKey types.ApiKey) error {
	sqlStatement := `
delete from api_key where key = $1 and owner = $2 and owner_type = $3
`
	_, err := d.pgDb.Exec(
		sqlStatement,
		apiKey.Key,
		apiKey.Owner,
		apiKey.OwnerType,
	)
	return err
}

func (d *PostgresStore) CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	var key types.ApiKey
	sqlStatement := `
select
	key, owner, owner_type
from
	api_key
where
	key = $1
`
	row := d.pgDb.QueryRow(sqlStatement, apiKey)
	err := row.Scan(&key.Key, &key.Owner, &key.OwnerType)
	if err != nil {
		if err == sql.ErrNoRows {
			// not an error, but not a valid api key either
			return nil, nil
		}
		return nil, err
	}
	return &key, nil
}

// Compile-time interface check:
var _ Store = (*PostgresStore)(nil)

func (d *PostgresStore) MigrateUp() error {
	migrations, err := d.GetMigrations()
	if err != nil {
		return err
	}
	err = migrations.Up()
	if err != migrate.ErrNoChange {
		return err
	}
	return nil
}

func (d *PostgresStore) MigrateDown() error {
	migrations, err := d.GetMigrations()
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

func (d *PostgresStore) GetMigrations() (*migrate.Migrate, error) {
	files, err := iofs.New(fs, "migrations")
	if err != nil {
		return nil, err
	}
	migrations, err := migrate.NewWithSourceInstance(
		"iofs",
		files,
		fmt.Sprintf("%s&&x-migrations-table=helix_schema_migrations", d.connectionString),
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

			sslSettings := "sslmode=disable"
			// crtPath := "/tmp/ca.crt"

			// TODO: enable
			// if c.Database.CaCrt != "" {
			// 	_, err = os.Stat(c.Database.CaCrt)
			// 	if err != nil {
			// 		err = os.WriteFile(crtPath, []byte(c.Database.CaCrt), 0644)
			// 		if err != nil {
			// 			return nil, fmt.Errorf("failed to write ca.crt: %w", err)
			// 		}
			// 	} else {
			// 		// File exists, so that's our path
			// 		crtPath = c.Database.CaCrt
			// 	}

			// 	sslSettings = fmt.Sprintf("sslmode=verify-full sslrootcert=%s", crtPath)
			// }

			dsn := fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s %s",
				cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database, sslSettings)
			dialector = postgres.Open(dsn)

			log.Info().Str("dsn", dsn).Msg("sql store connecting to DB")

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

			log.Info().Str("dsn", dsn).Msg("sql store connected")

			// success
			return db, nil
		}
	}
}
