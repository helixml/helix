// Package leader runs work that must have a single owner across every API
// process sharing a database: cron schedulers, bot gateway connections,
// orchestration loops and the startup resets that fail work a dead process
// abandoned. During a blue/green deploy two API processes serve at once; only
// the one holding the lease runs this work.
//
// The lease is a session-level Postgres advisory lock held on a dedicated
// connection. Postgres releases it when the process exits or the connection
// drops, so a crashed leader can't block a successor.
package leader

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// DefaultInterval is how often a follower retries the lock and a leader
// checks its connection.
const DefaultInterval = 2 * time.Second

// Elector campaigns for the lease and, while it holds it, runs the registered
// tasks.
type Elector struct {
	db       *sql.DB
	key      string
	interval time.Duration

	mu        sync.Mutex
	tasks     []task
	leaderCtx context.Context // nil while not leader
	running   sync.WaitGroup
	active    map[string]int // running tasks by name, guarded by mu
	leader    atomic.Bool
}

type task struct {
	name string
	fn   func(ctx context.Context)
}

// New returns an Elector for the lease named key on db. Every process that
// shares the work must use the same key.
func New(db *sql.DB, key string) *Elector {
	return &Elector{db: db, key: key, interval: DefaultInterval}
}

// SetInterval overrides DefaultInterval (tests).
func (e *Elector) SetInterval(d time.Duration) { e.interval = d }

// IsLeader reports whether this process holds the lease.
func (e *Elector) IsLeader() bool { return e.leader.Load() }

// Go registers fn to run each time this process becomes leader. fn gets a
// context that is cancelled when leadership is lost, and must return once it
// is: the lease is not campaigned for again until every task has returned. A
// task that only does startup work returns when that is done. Registering
// while leader starts fn at once.
func (e *Elector) Go(name string, fn func(ctx context.Context)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := task{name: name, fn: fn}
	e.tasks = append(e.tasks, t)
	if e.leaderCtx != nil {
		e.start(e.leaderCtx, t)
	}
}

// start runs t in its own goroutine. The caller holds e.mu.
func (e *Elector) start(ctx context.Context, t task) {
	e.running.Add(1)
	if e.active == nil {
		e.active = map[string]int{}
	}
	e.active[t.name]++
	go func() {
		defer e.running.Done()
		defer func() {
			e.mu.Lock()
			if e.active[t.name]--; e.active[t.name] == 0 {
				delete(e.active, t.name)
			}
			e.mu.Unlock()
		}()
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("task", t.name).Interface("panic", r).Msg("leader task panicked")
			}
		}()
		t.fn(ctx)
	}()
}

// Run campaigns for the lease until ctx is done, running the registered tasks
// while it holds it.
func (e *Elector) Run(ctx context.Context) {
	for {
		held, err := e.lead(ctx)
		if err != nil && ctx.Err() == nil {
			log.Warn().Err(err).Str("lease", e.key).Msg("leader lease lost")
		}
		if held {
			log.Info().Str("lease", e.key).Msg("leadership released")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(e.interval):
		}
	}
}

// lead takes the lease if it is free, then holds it until ctx is done or the
// connection fails. held reports whether it was taken.
func (e *Elector) lead(ctx context.Context) (held bool, err error) {
	conn, err := e.db.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("get lease connection: %w", err)
	}
	healthy := false
	defer func() {
		if healthy {
			// Unlock before the connection goes back to the pool.
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, unlockErr := conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock(hashtext($1))", e.key)
			cancel()
			healthy = unlockErr == nil
		}
		if !healthy {
			// Discard the connection: if it still holds the lock, closing
			// the session is what releases it.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
	}()

	var acquired bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtext($1))", e.key).Scan(&acquired); err != nil {
		return false, fmt.Errorf("try lease: %w", err)
	}
	healthy = true
	if !acquired {
		return false, nil
	}

	// A predecessor whose session died (network partition, Postgres
	// restart) lost the lock at once but only notices at its next check, up
	// to two intervals later. Wait that out before starting the tasks, so
	// that its tasks have been cancelled before ours start.
	if err := e.hold(ctx, conn, time.Now().Add(e.takeoverDelay())); err != nil {
		healthy = false
		return false, err
	}
	if ctx.Err() != nil {
		return false, nil
	}

	log.Info().Str("lease", e.key).Msg("became leader")
	leaderCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.leaderCtx = leaderCtx
	e.leader.Store(true)
	for _, t := range e.tasks {
		e.start(leaderCtx, t)
	}
	e.mu.Unlock()

	err = e.hold(leaderCtx, conn, time.Time{})
	if err != nil {
		healthy = false
	}

	e.mu.Lock()
	e.leader.Store(false)
	e.leaderCtx = nil
	cancel()
	e.mu.Unlock()
	e.waitTasks()
	return true, err
}

// takeoverDelay is how long a new leader waits before starting its tasks: a
// predecessor detects a dead session within two intervals (one tick plus a
// ping that times out after one interval).
func (e *Elector) takeoverDelay() time.Duration { return 3 * e.interval }

// hold checks the lease connection every interval until ctx is done, until
// (when not zero) is reached, or the check fails.
func (e *Elector) hold(ctx context.Context, conn *sql.Conn, until time.Time) error {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		if !until.IsZero() && !time.Now().Before(until) {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		pingCtx, cancel := context.WithTimeout(ctx, e.interval)
		err := conn.PingContext(pingCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("lease connection: %w", err)
		}
	}
}

// waitTasks waits for the tasks of the leadership that just ended, logging
// while any is slow to stop.
// Running names the leader tasks that have not returned yet, sorted.
func (e *Elector) Running() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	names := make([]string, 0, len(e.active))
	for name := range e.active {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e *Elector) waitTasks() {
	done := make(chan struct{})
	go func() {
		e.running.Wait()
		close(done)
	}()
	for {
		select {
		case <-done:
			return
		case <-time.After(30 * time.Second):
			log.Warn().Str("lease", e.key).Strs("tasks", e.Running()).Msg("waiting for leader tasks to stop")
		}
	}
}
