package slack

import (
	"context"
	"database/sql"
	"log/slog"
)

// Locker is an exclusive, cross-replica lock. TryLock returns
// acquired=true only for the single caller that currently holds it;
// Unlock releases it. The production implementation is a Postgres
// advisory lock (one connection holds the session lock); tests use a
// fake shared lockbox.
type Locker interface {
	TryLock(ctx context.Context) (acquired bool, err error)
	Unlock(ctx context.Context) error
}

// SingleOwner gates the Socket Mode connection so that, across a
// multi-replica deployment, exactly one replica opens the single
// outbound WebSocket and runs ingest (NFR-2). The winner holds the
// lock; losers poll to take over on failover (NFR-5).
type SingleOwner struct {
	locker Locker
	logger *slog.Logger
}

// NewSingleOwner wraps a Locker.
func NewSingleOwner(locker Locker, logger *slog.Logger) *SingleOwner {
	if logger == nil {
		logger = slog.Default()
	}
	return &SingleOwner{locker: locker, logger: logger}
}

// TryAcquire reports whether this replica won (or already holds) the
// lock. A lock error is treated as "not acquired" and logged — a replica
// that can't reach the lock must not assume ownership.
func (o *SingleOwner) TryAcquire(ctx context.Context) bool {
	ok, err := o.locker.TryLock(ctx)
	if err != nil {
		o.logger.Warn("slack.singleowner: try-lock", "err", err)
		return false
	}
	return ok
}

// Release frees the lock so another replica can take over.
func (o *SingleOwner) Release(ctx context.Context) {
	if err := o.locker.Unlock(ctx); err != nil {
		o.logger.Warn("slack.singleowner: unlock", "err", err)
	}
}

// SocketModeLockKey is the constant advisory-lock key the Socket Mode
// owner contends on. Arbitrary but fixed across replicas so they all
// fight over the same lock.
const SocketModeLockKey int64 = 0x5_1ACC_50C7 // "slack soc(ket)"

// PgAdvisoryLock is the production Locker: a Postgres session-level
// advisory lock held on one dedicated connection. pg_try_advisory_lock
// is non-blocking — it returns immediately with whether the lock was
// taken — which is exactly the poll-and-take-over semantics SingleOwner
// wants.
type PgAdvisoryLock struct {
	db   *sql.DB
	key  int64
	conn *sql.Conn
}

// NewPgAdvisoryLock builds a Postgres advisory lock on the given key.
func NewPgAdvisoryLock(db *sql.DB, key int64) *PgAdvisoryLock {
	return &PgAdvisoryLock{db: db, key: key}
}

// TryLock attempts pg_try_advisory_lock on a fresh dedicated connection.
// On success the connection is retained (the session lock lives on it);
// on failure the connection is returned to the pool. Re-calling while
// already holding is a no-op success.
func (p *PgAdvisoryLock) TryLock(ctx context.Context) (bool, error) {
	if p.conn != nil {
		return true, nil
	}
	conn, err := p.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", p.key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return false, err
	}
	if !acquired {
		_ = conn.Close()
		return false, nil
	}
	p.conn = conn
	return true, nil
}

// Unlock releases the advisory lock and returns the dedicated
// connection to the pool. No-op when not held.
func (p *PgAdvisoryLock) Unlock(ctx context.Context) error {
	if p.conn == nil {
		return nil
	}
	_, err := p.conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", p.key)
	_ = p.conn.Close()
	p.conn = nil
	return err
}

// compile-time assertion.
var _ Locker = (*PgAdvisoryLock)(nil)
