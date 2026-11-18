package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4/database"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

// Migrations run while the previous release keeps serving (blue/green
// deploys) and must never queue behind a lock for long: a waiting ACCESS
// EXCLUSIVE request blocks every reader that arrives after it. On 2026-11-18
// an ALTER that waited behind a pg_dump took app.helix.ml down for half an
// hour. Every migration connection therefore runs with lock_timeout, a step
// that hits it is retried a bounded number of times, and then the migration
// fails, naming the sessions that held the lock.
const (
	migrationLockTimeout = 5 * time.Second
	// lockTimeoutSQLState is lock_not_available, raised when lock_timeout expires.
	lockTimeoutSQLState = "55P03"
	// backfillBatchSize bounds the rows one backfill statement updates, so no
	// transaction holds row locks for long.
	backfillBatchSize = 1000
)

// lockTimeoutRetryDelays are the waits between attempts of a migration step
// that hit lock_timeout: five attempts within about 40s, after which the step
// fails.
var lockTimeoutRetryDelays = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// isLockTimeout reports whether err is Postgres' lock_not_available, from
// GORM (pgx) or golang-migrate (lib/pq).
func isLockTimeout(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == lockTimeoutSQLState
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == lockTimeoutSQLState
	}
	// golang-migrate wraps driver errors in database.Error, which has no Unwrap.
	var dbErr *database.Error
	if errors.As(err, &dbErr) {
		return isLockTimeout(dbErr.OrigErr)
	}
	return false
}

// migrationStep runs one idempotent migration step, retrying it while it hits
// lock_timeout.
func (s *PostgresStore) migrationStep(name string, step func() error) error {
	for attempt := 0; ; attempt++ {
		err := step()
		if err == nil {
			return nil
		}
		if !isLockTimeout(err) {
			return fmt.Errorf("migration step %s: %w", name, err)
		}
		blockers := s.lockBlockers()
		if attempt == len(lockTimeoutRetryDelays) {
			return fmt.Errorf("migration step %s blocked by a lock after %d attempts (oldest open transactions: %s): %w",
				name, attempt+1, blockers, err)
		}
		log.Warn().Str("step", name).Int("attempt", attempt+1).Str("oldest_open_transactions", blockers).
			Dur("retry_in", lockTimeoutRetryDelays[attempt]).
			Msg("migration step hit lock_timeout; retrying")
		time.Sleep(lockTimeoutRetryDelays[attempt])
	}
}

// lockBlockers lists the oldest open transactions in the database, for the
// error a blocked migration step reports. Once lock_timeout cancels the step
// nothing waits any more, so pg_blocking_pids can't name the holder; a
// long-open transaction (a pg_dump, a stuck query) is what blocks DDL.
func (s *PostgresStore) lockBlockers() string {
	var rows []struct {
		PID   int
		State string
		Age   string
		Query string
	}
	err := s.gdb.Raw(`
		SELECT pid, state, date_trunc('second', now() - xact_start)::text AS age, left(query, 200) AS query
		FROM pg_stat_activity
		WHERE datname = current_database() AND pid <> pg_backend_pid() AND xact_start IS NOT NULL
		ORDER BY xact_start
		LIMIT 5`).Scan(&rows).Error
	if err != nil {
		return "unknown: " + err.Error()
	}
	if len(rows) == 0 {
		return "none open now"
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = fmt.Sprintf("pid %d (%s, open %s): %s", r.PID, r.State, r.Age, r.Query)
	}
	return strings.Join(out, "; ")
}

// exists reports whether query returns a row. Backfills probe with it first
// so that an already-migrated database sees reads only.
func (s *PostgresStore) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var found bool
	if err := s.gdb.WithContext(ctx).Raw("SELECT EXISTS ("+query+")", args...).Scan(&found).Error; err != nil {
		return false, err
	}
	return found, nil
}

// execBatches runs stmt, which must update at most backfillBatchSize rows
// that then no longer match it, until a run updates fewer. Each run is its
// own transaction.
func (s *PostgresStore) execBatches(ctx context.Context, stmt string, args ...any) error {
	for {
		res := s.gdb.WithContext(ctx).Exec(stmt, args...)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected < backfillBatchSize {
			return nil
		}
	}
}

// backfillRows sets columns on the rows of table matching where, in batches
// by id. It reads first and writes nothing when no row matches. set must make
// a row stop matching where.
func (s *PostgresStore) backfillRows(ctx context.Context, table, set string, setArgs []any, where string, whereArgs ...any) error {
	found, err := s.exists(ctx, fmt.Sprintf("SELECT 1 FROM %s WHERE %s", table, where), whereArgs...)
	if err != nil || !found {
		return err
	}
	args := append(append([]any{}, setArgs...), whereArgs...)
	return s.execBatches(ctx, fmt.Sprintf(
		"UPDATE %s SET %s WHERE id IN (SELECT id FROM %s WHERE %s LIMIT %d)",
		table, set, table, where, backfillBatchSize), args...)
}

// columnDefault returns a column's catalog default expression ("" when it has
// none).
func (s *PostgresStore) columnDefault(ctx context.Context, table, column string) (string, error) {
	var def *string
	err := s.gdb.WithContext(ctx).Raw(`
		SELECT column_default FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, column).Scan(&def).Error
	if err != nil || def == nil {
		return "", err
	}
	return *def, nil
}

// columnNullable reports whether a column accepts NULL.
func (s *PostgresStore) columnNullable(ctx context.Context, table, column string) (bool, error) {
	var nullable string
	err := s.gdb.WithContext(ctx).Raw(`
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`, table, column).Scan(&nullable).Error
	return nullable == "YES", err
}

// createIndexConcurrently builds index name with ddl (a CREATE [UNIQUE]
// INDEX CONCURRENTLY statement) unless a valid index of that name exists. An
// invalid one, left by an interrupted build, is dropped and rebuilt.
// CONCURRENTLY takes no lock that blocks writes, so the previous release keeps
// serving while it builds.
func (s *PostgresStore) createIndexConcurrently(ctx context.Context, name, ddl string) error {
	var state struct {
		Present bool
		Valid   bool
	}
	err := s.gdb.WithContext(ctx).Raw(`
		SELECT to_regclass(?) IS NOT NULL AS present,
		       COALESCE((SELECT indisvalid FROM pg_index WHERE indexrelid = to_regclass(?)), false) AS valid`,
		name, name).Scan(&state).Error
	if err != nil {
		return fmt.Errorf("inspect index %s: %w", name, err)
	}
	if state.Valid {
		return nil
	}
	if state.Present {
		if err := s.gdb.WithContext(ctx).Exec("DROP INDEX CONCURRENTLY " + name).Error; err != nil {
			return fmt.Errorf("drop invalid index %s: %w", name, err)
		}
	}
	if err := s.gdb.WithContext(ctx).Exec(ddl).Error; err != nil {
		return fmt.Errorf("create index %s: %w", name, err)
	}
	return nil
}
