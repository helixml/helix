package gorm_test

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
)

// TestDomainEventsAppendAndListBySubject exercises the gorm-backed
// domain-event store end to end against a real DB — in particular the
// store.Option composition ListBySubject relies on: the (org, type,
// subject) equalities, the optional created_at window (WithWhere), and the
// newest-first ordering. Mirrors the in-memory contract test.
func TestDomainEventsAppendAndListBySubject(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)

	mk := func(id, org, subject, worker string, at time.Time) domainevent.DomainEvent {
		e, err := domainevent.New(id, org, domainevent.TypeSlackThreadParticipant, subject, worker, "p-router", nil, at)
		if err != nil {
			t.Fatalf("new %s: %v", id, err)
		}
		return e
	}
	for _, e := range []domainevent.DomainEvent{
		mk("d-1", "org-1", "T", "w-alice", base),
		mk("d-2", "org-1", "T", "w-bob", base.Add(time.Minute)),
		mk("d-3", "org-1", "U", "w-carol", base), // other subject — noise
		mk("d-4", "org-2", "T", "w-dave", base),  // other org — noise
	} {
		if err := s.DomainEvents.Append(ctx, e); err != nil {
			t.Fatalf("append %s: %v", e.ID, err)
		}
	}

	// All of thread T in org-1, newest first.
	got, err := s.DomainEvents.ListBySubject(ctx, "org-1", domainevent.TypeSlackThreadParticipant, "T", time.Time{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if parts := domainevent.Participants(got); len(parts) != 2 || parts[0] != "w-bob" || parts[1] != "w-alice" {
		t.Fatalf("want [w-bob w-alice] (newest first, org+subject scoped), got %v", parts)
	}

	// created_at window (WithWhere) excludes the older event.
	recent, err := s.DomainEvents.ListBySubject(ctx, "org-1", domainevent.TypeSlackThreadParticipant, "T", base.Add(30*time.Second))
	if err != nil {
		t.Fatalf("list windowed: %v", err)
	}
	if parts := domainevent.Participants(recent); len(parts) != 1 || parts[0] != "w-bob" {
		t.Fatalf("window: want [w-bob], got %v", parts)
	}

	// Empty result is not an error.
	none, err := s.DomainEvents.ListBySubject(ctx, "org-1", domainevent.TypeSlackThreadParticipant, "does-not-exist", time.Time{})
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("want no rows for unknown subject, got %d", len(none))
	}
}
