package store

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"time"

	"database/sql"

	_ "github.com/lib/pq"

	sync "github.com/bacalhau-project/golang-mutex-tracer"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type PostgresStore struct {
	mtx              sync.RWMutex
	options          StoreOptions
	connectionString string
	db               *sql.DB
}

func NewPostgresStore(
	options StoreOptions,
) (*PostgresStore, error) {
	connectionString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		options.Username,
		options.Password,
		options.Host,
		options.Port,
		options.Database,
	)
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, err
	}
	store := &PostgresStore{
		connectionString: connectionString,
		options:          options,
		db:               db,
	}
	store.mtx.EnableTracerWithOpts(sync.Opts{
		Threshold: 10 * time.Millisecond,
		Id:        "PostgresStore.mtx",
	})
	if options.AutoMigrate {
		err = store.MigrateUp()
		if err != nil {
			return nil, fmt.Errorf("there was an error doing the migration: %s", err.Error())
		}
	}
	return store, nil
}

func (d *PostgresStore) DeleteSession(
	ctx context.Context,
	sessionID string,
) (*types.Session, error) {
	deleted, err := d.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	d.mtx.Lock()
	defer d.mtx.Unlock()
	_, err = d.db.Exec(`
		DELETE FROM session WHERE id = $1
	`, sessionID)
	if err != nil {
		return nil, err
	}

	return deleted, nil
}

func (d *PostgresStore) GetSession(
	ctx context.Context,
	sessionID string,
) (*types.Session, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}
	row := d.db.QueryRow(`
		SELECT id, name, parent_session, mode, type, model_name, finetune_file, interactions, owner, owner_type
		FROM session WHERE id = $1
	`, sessionID)

	var interactions []byte
	session := &types.Session{}
	err := row.Scan(&session.ID, &session.Name, &session.ParentSession, &session.Mode, &session.Type, &session.ModelName, &session.FinetuneFile, &interactions, &session.Owner, &session.OwnerType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	err = json.Unmarshal(interactions, &session.Interactions)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (d *PostgresStore) GetSessions(
	ctx context.Context,
	query GetSessionsQuery,
) ([]*types.Session, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	var rows *sql.Rows
	var err error

	/// XXX SECURITY not sure this is what we want - audit who can set these values?
	if query.Owner != "" && query.OwnerType != "" {
		rows, err = d.db.Query(`
			SELECT id, created, updated, name, parent_session, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE owner = $1 AND owner_type = $2 AND parent_session IS NULL
			ORDER BY created DESC
		`, query.Owner, query.OwnerType)
	} else if query.Owner != "" {
		rows, err = d.db.Query(`
			SELECT id, created, updated, name, parent_session, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE owner = $1
			ORDER BY created DESC
		`, query.Owner)
	} else if query.OwnerType != "" {
		rows, err = d.db.Query(`
			SELECT id, created, updated, name, parent_session, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE owner_type = $1 AND parent_session IS NULL
			ORDER BY created DESC
		`, query.OwnerType)
	} else {
		rows, err = d.db.Query(`
			SELECT id, created, updated, name, parent_session, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE parent_session IS NULL
			ORDER BY created DESC
		`)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []*types.Session{}
	for rows.Next() {
		session := &types.Session{}

		var interactions []byte
		err := rows.Scan(&session.ID, &session.Created, &session.Updated, &session.Name, &session.ParentSession, &session.Mode, &session.Type, &session.ModelName, &session.FinetuneFile, &interactions, &session.Owner, &session.OwnerType)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(interactions, &session.Interactions)
		if err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (d *PostgresStore) CreateSession(
	ctx context.Context,
	session types.Session,
) (*types.Session, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	interactions, err := json.Marshal(session.Interactions)
	if err != nil {
		return nil, err
	}

	_, err = d.db.Exec(`
		INSERT INTO session (
			id, name, parent_session, mode, type, model_name, finetune_file, interactions, owner, owner_type
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`, session.ID, session.Name, session.ParentSession, session.Mode, session.Type, session.ModelName, session.FinetuneFile, interactions, session.Owner, session.OwnerType)

	if err != nil {
		return nil, err
	}

	return &session, nil
}

func (d *PostgresStore) UpdateSession(
	ctx context.Context,
	session types.Session,
) (*types.Session, error) {
	// TODO: think about which of these fields are meant to be mutable
	d.mtx.Lock()
	defer d.mtx.Unlock()

	interactions, err := json.Marshal(session.Interactions)
	if err != nil {
		return nil, err
	}

	_, err = d.db.Exec(`
		UPDATE session SET
			name = $2,
			parent_session = $3,
			mode = $4,
			type = $5,
			model_name = $6,
			finetune_file = $7,
			interactions = $8,
			owner = $9,
			owner_type = $10
		WHERE id = $1
	`, session.ID, session.Name, session.ParentSession, session.Mode, session.Type, session.ModelName, session.FinetuneFile, interactions, session.Owner, session.OwnerType)

	if err != nil {
		return nil, err
	}

	// TODO maybe do a SELECT to get the exact session that's in the database
	return &session, nil
}

func (d *PostgresStore) GetBalanceTransfers(
	ctx context.Context,
	query OwnerQuery,
) ([]*types.BalanceTransfer, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	var transfers []*types.BalanceTransfer
	var rows *sql.Rows
	var err error

	rows, err = d.db.Query(`
		SELECT
			id, created, owner, owner_type, payment_type, amount, data
		FROM
			balance_transfer
		WHERE
			owner = $1 AND owner_type = $2
		ORDER BY
			created ASC
	`, query.Owner, query.OwnerType)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		var created time.Time
		var owner string
		var ownerType types.OwnerType
		var paymentType types.PaymentType
		var amount int
		var data []byte

		err = rows.Scan(&id, &created, &owner, &ownerType, &paymentType, &amount, &data)
		if err != nil {
			return nil, err
		}

		var transferData types.BalanceTransferData
		err = json.Unmarshal(data, &transferData)
		if err != nil {
			return nil, err
		}

		transfer := &types.BalanceTransfer{
			ID:          id,
			Created:     created,
			Owner:       owner,
			OwnerType:   ownerType,
			PaymentType: paymentType,
			Amount:      amount,
			Data:        transferData,
		}

		transfers = append(transfers, transfer)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return transfers, nil
}

func (d *PostgresStore) CreateBalanceTransfer(
	ctx context.Context,
	transfer types.BalanceTransfer,
) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	transferData, err := json.Marshal(transfer.Data)
	if err != nil {
		return err
	}
	sqlStatement := `
insert into
balance_transfer (
	id,
	owner,
	owner_type,
	payment_type,
	amount,
	data
)
values ($1, $2, $3, $4, $5, $6)`
	_, err = d.db.Exec(
		sqlStatement,
		transfer.ID,
		transfer.Owner,
		transfer.OwnerType,
		transfer.PaymentType,
		transfer.Amount,
		transferData,
	)
	if err != nil {
		return err
	}
	return nil
}

func (d *PostgresStore) CreateAPIKey(ctx context.Context, owner OwnerQuery, name string) (string, error) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

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
	err = d.db.QueryRow(
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
	return "lp-" + base64.URLEncoding.EncodeToString(key), nil
}

func (d *PostgresStore) GetAPIKeys(ctx context.Context, query OwnerQuery) ([]*types.ApiKey, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
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
	rows, err := d.db.Query(
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
	d.mtx.Lock()
	defer d.mtx.Unlock()
	sqlStatement := `
delete from api_key where key = $1 and owner = $2 and owner_type = $3
`
	_, err := d.db.Exec(
		sqlStatement,
		apiKey.Key,
		apiKey.Owner,
		apiKey.OwnerType,
	)
	return err
}

func (d *PostgresStore) CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	var key types.ApiKey
	sqlStatement := `
select
	key, owner, owner_type
from
	api_key
where
	key = $1
`
	row := d.db.QueryRow(sqlStatement, apiKey)
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
