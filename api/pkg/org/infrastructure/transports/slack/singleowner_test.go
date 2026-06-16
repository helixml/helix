// Single-owner tests (NFR-2): the advisory-lock gate ensures exactly
// one replica owns the Socket Mode connection. The Postgres
// implementation is exercised in production; here a fake shared lockbox
// stands in so the contention logic is tested without a database.
package slack_test

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"

	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

// lockbox is a shared lock two fakeLockers contend over.
type lockbox struct {
	mu   sync.Mutex
	held bool
}

type fakeLocker struct {
	box     *lockbox
	holding bool
}

func (l *fakeLocker) TryLock(context.Context) (bool, error) {
	l.box.mu.Lock()
	defer l.box.mu.Unlock()
	if l.holding {
		return true, nil
	}
	if l.box.held {
		return false, nil
	}
	l.box.held = true
	l.holding = true
	return true, nil
}

func (l *fakeLocker) Unlock(context.Context) error {
	l.box.mu.Lock()
	defer l.box.mu.Unlock()
	if l.holding {
		l.box.held = false
		l.holding = false
	}
	return nil
}

func newOwner(box *lockbox) *slacktransport.SingleOwner {
	return slacktransport.NewSingleOwner(&fakeLocker{box: box}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestSingleOwner_OnlyOneAcquires(t *testing.T) {
	box := &lockbox{}
	a := newOwner(box)
	b := newOwner(box)
	ctx := context.Background()

	if !a.TryAcquire(ctx) {
		t.Fatalf("first contender failed to acquire")
	}
	if b.TryAcquire(ctx) {
		t.Fatalf("second contender acquired while first holds the lock")
	}
}

func TestSingleOwner_TakesOverAfterRelease(t *testing.T) {
	box := &lockbox{}
	a := newOwner(box)
	b := newOwner(box)
	ctx := context.Background()

	if !a.TryAcquire(ctx) {
		t.Fatalf("first contender failed to acquire")
	}
	a.Release(ctx)
	if !b.TryAcquire(ctx) {
		t.Fatalf("second contender failed to take over after release")
	}
}
