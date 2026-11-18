package leader

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// openDB connects to the Postgres that POSTGRES_* configures (as CI's
// unit-test step does). Leases only take advisory locks on a key unique to
// the test, so the configured database itself is safe to use.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		t.Skip("POSTGRES_HOST not set")
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD")),
		Host:     net.JoinHostPort(host, port),
		Path:     os.Getenv("POSTGRES_DATABASE"),
		RawQuery: "sslmode=disable",
	}
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// contender is one simulated API process.
type contender struct {
	elector *Elector
	cancel  context.CancelFunc
	done    chan struct{}
	starts  atomic.Int32
}

func newContender(t *testing.T, db *sql.DB, key string, active *atomic.Int32, overlap *atomic.Bool) *contender {
	c := &contender{elector: New(db, key), done: make(chan struct{})}
	c.elector.SetInterval(50 * time.Millisecond)
	c.elector.Go("singleton", func(ctx context.Context) {
		c.starts.Add(1)
		if active.Add(1) > 1 {
			overlap.Store(true)
		}
		<-ctx.Done()
		active.Add(-1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go func() {
		c.elector.Run(ctx)
		close(c.done)
	}()
	t.Cleanup(c.stop)
	return c
}

func (c *contender) stop() {
	c.cancel()
	<-c.done
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestOnlyOneContenderLeadsAndTheOtherTakesOver(t *testing.T) {
	db := openDB(t)
	key := fmt.Sprintf("leader-test-%d", time.Now().UnixNano())
	var active atomic.Int32
	var overlap atomic.Bool

	a := newContender(t, db, key, &active, &overlap)
	waitFor(t, "a to lead", a.elector.IsLeader)
	b := newContender(t, db, key, &active, &overlap)

	time.Sleep(500 * time.Millisecond) // ten of b's campaign intervals
	if b.elector.IsLeader() || b.starts.Load() != 0 {
		t.Fatal("b became leader while a held the lease")
	}

	a.stop()
	if a.elector.IsLeader() {
		t.Fatal("a still reports leadership after stopping")
	}
	waitFor(t, "b to take over", b.elector.IsLeader)
	waitFor(t, "b's task to start", func() bool { return b.starts.Load() == 1 })
	if overlap.Load() {
		t.Fatal("the singleton task ran in both contenders at once")
	}
}

func TestLeaseIsReleasedWhenTheConnectionDies(t *testing.T) {
	db := openDB(t)
	key := fmt.Sprintf("leader-test-%d", time.Now().UnixNano())
	var active atomic.Int32
	var overlap atomic.Bool

	a := newContender(t, db, key, &active, &overlap)
	waitFor(t, "a to lead", a.elector.IsLeader)
	b := newContender(t, db, key, &active, &overlap)

	// Kill a's lease session, as a network partition or Postgres restart
	// would. a must notice, stop its task and stop claiming leadership; the
	// session's death frees the lock for b.
	var killed bool
	if err := db.QueryRow(`
		SELECT pg_terminate_backend(pid) FROM pg_locks
		WHERE locktype = 'advisory' AND granted AND objid = hashtext($1)::oid`, key).Scan(&killed); err != nil || !killed {
		t.Fatalf("terminate lease session: killed=%v err=%v", killed, err)
	}

	waitFor(t, "b to take over", func() bool { return b.elector.IsLeader() && b.starts.Load() == 1 })
	waitFor(t, "a to step down", func() bool { return !a.elector.IsLeader() })
	if overlap.Load() {
		t.Fatal("the singleton task ran in both contenders at once")
	}
}

func TestTaskRegisteredWhileLeaderStartsAtOnce(t *testing.T) {
	db := openDB(t)
	e := New(db, fmt.Sprintf("leader-test-%d", time.Now().UnixNano()))
	e.SetInterval(50 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	waitFor(t, "leadership", e.IsLeader)

	var wg sync.WaitGroup
	wg.Add(1)
	ran := make(chan struct{})
	e.Go("late", func(_ context.Context) {
		close(ran)
		wg.Done()
	})
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("task registered while leader did not start")
	}
	wg.Wait()
}
