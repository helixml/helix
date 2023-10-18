package store

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"

	"time"

	"database/sql"

	_ "github.com/lib/pq"

	sync "github.com/bacalhau-project/golang-mutex-tracer"
	"github.com/bacalhau-project/lilysaas/api/pkg/types"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
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
		SELECT id, name, mode, type, model_name, finetune_file, interactions, owner, owner_type
		FROM session WHERE id = $1
	`, sessionID)

	var interactions []byte
	session := &types.Session{}
	err := row.Scan(&session.ID, &session.Name, &session.Mode, &session.Type, &session.ModelName, &session.FinetuneFile, &interactions, &session.Owner, &session.OwnerType)
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
			SELECT id, name, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE owner = $1 AND owner_type = $2
			ORDER BY created DESC
		`, query.Owner, query.OwnerType)
	} else if query.Owner != "" {
		rows, err = d.db.Query(`
			SELECT id, name, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE owner = $1
			ORDER BY created DESC
		`, query.Owner)
	} else if query.OwnerType != "" {
		rows, err = d.db.Query(`
			SELECT id, name, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
			WHERE owner_type = $1
			ORDER BY created DESC
		`, query.OwnerType)
	} else {
		rows, err = d.db.Query(`
			SELECT id, name, mode, type, model_name, finetune_file, interactions, owner, owner_type
			FROM session
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
		err := rows.Scan(&session.ID, &session.Name, &session.Mode, &session.Type, &session.ModelName, &session.FinetuneFile, &interactions, &session.Owner, &session.OwnerType)
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
			id, name, mode, type, model_name, finetune_file, interactions, owner, owner_type
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`, session.ID, session.Name, session.Mode, session.Type, session.ModelName, session.FinetuneFile, interactions, session.Owner, session.OwnerType)

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
			mode = $3,
			type = $4,
			model_name = $5,
			finetune_file = $6,
			interactions = $7,
			owner = $8,
			owner_type = $9
		WHERE id = $1
	`, session.ID, session.Name, session.Mode, session.Type, session.ModelName, session.FinetuneFile, interactions, session.Owner, session.OwnerType)

	if err != nil {
		return nil, err
	}

	// TODO maybe do a SELECT to get the exact session that's in the database
	return &session, nil
}

func (d *PostgresStore) GetJobs(
	ctx context.Context,
	query GetJobsQuery,
) ([]*types.Job, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()

	var jobs []*types.Job
	var rows *sql.Rows
	var err error

	rows, err = d.db.Query(`
		SELECT
			id, created, owner, owner_type, state, status, data
		FROM
			job
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
		var state string
		var status string
		var data []byte

		err = rows.Scan(&id, &created, &owner, &ownerType, &state, &status, &data)
		if err != nil {
			return nil, err
		}

		var jobData types.JobData
		err = json.Unmarshal(data, &jobData)
		if err != nil {
			return nil, err
		}

		job := &types.Job{
			ID:        id,
			Created:   created,
			Owner:     owner,
			OwnerType: ownerType,
			State:     state,
			Status:    status,
			Data:      jobData,
		}

		jobs = append(jobs, job)
	}

	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

func (d *PostgresStore) GetBalanceTransfers(
	ctx context.Context,
	query GetBalanceTransfersQuery,
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

func (d *PostgresStore) GetJob(
	ctx context.Context,
	queryID string,
) (*types.Job, error) {
	d.mtx.RLock()
	defer d.mtx.RUnlock()
	var id string
	var created time.Time
	var owner string
	var ownerType types.OwnerType
	var state string
	var status string
	var data []byte
	row := d.db.QueryRow(`
select
	id, created, owner, owner_type, state, status, data
from
	job
where
	id = $1
limit 1
`, queryID)
	err := row.Scan(&id, &created, &owner, &ownerType, &state, &status, &data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		} else {
			return nil, err
		}
	}
	var jobData types.JobData
	err = json.Unmarshal(data, &jobData)
	if err != nil {
		return nil, err
	}
	return &types.Job{
		ID:        id,
		Created:   created,
		Owner:     owner,
		OwnerType: ownerType,
		State:     state,
		Status:    status,
		Data:      jobData,
	}, nil
}

func (d *PostgresStore) CreateJob(
	ctx context.Context,
	job types.Job,
) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	jobData, err := json.Marshal(job.Data)
	if err != nil {
		return err
	}
	sqlStatement := `
insert into
job (
	id,
	owner,
	owner_type,
	state,
	status,
	data
)
values ($1, $2, $3, $4, $5, $6)`
	_, err = d.db.Exec(
		sqlStatement,
		job.ID,
		job.Owner,
		job.OwnerType,
		job.State,
		job.Status,
		jobData,
	)
	if err != nil {
		return err
	}
	return nil
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

func (d *PostgresStore) UpdateJob(
	ctx context.Context,
	id string,
	state string,
	status string,
	data types.JobData,
) error {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	jobData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	sqlStatement := `
update
	job
set
	state = $1,
	status = $2,
	data = $3
where
	id = $4
`
	_, err = d.db.Exec(
		sqlStatement,
		state,
		status,
		jobData,
		id,
	)
	return err
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
		fmt.Sprintf("%s&&x-migrations-table=lilysaas_schema_migrations", d.connectionString),
	)
	if err != nil {
		return nil, err
	}
	return migrations, nil
}
