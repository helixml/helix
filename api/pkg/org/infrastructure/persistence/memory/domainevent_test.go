package memory

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/domainevent"
)

func TestDomainEventsAppendAndListBySubject(t *testing.T) {
	ctx := context.Background()
	s := New()
	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)

	mk := func(id, org, subject, worker string, at time.Time) domainevent.DomainEvent {
		e, err := domainevent.New(id, org, domainevent.TypeSlackThreadParticipant, subject, worker, "p-router", nil, at)
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		return e
	}
	// Two participants in thread T, one in thread U, one in another org.
	for _, e := range []domainevent.DomainEvent{
		mk("d1", "org-1", "T", "w-alice", base),
		mk("d2", "org-1", "T", "w-bob", base.Add(time.Minute)),
		mk("d3", "org-1", "U", "w-carol", base),
		mk("d4", "org-2", "T", "w-dave", base),
	} {
		if err := s.DomainEvents.Append(ctx, e); err != nil {
			t.Fatalf("append: %v", err)
		}
	}

	got, err := s.DomainEvents.ListBySubject(ctx, "org-1", domainevent.TypeSlackThreadParticipant, "T", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	parts := domainevent.Participants(got)
	if len(parts) != 2 || parts[0] != "w-bob" || parts[1] != "w-alice" {
		t.Fatalf("want [w-bob w-alice] (newest first), got %v", parts)
	}

	// Time window excludes the older event.
	recent, _ := s.DomainEvents.ListBySubject(ctx, "org-1", domainevent.TypeSlackThreadParticipant, "T", base.Add(30*time.Second))
	if p := domainevent.Participants(recent); len(p) != 1 || p[0] != "w-bob" {
		t.Fatalf("window: want [w-bob], got %v", p)
	}
}
