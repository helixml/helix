package gorm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

func TestStreamsRoundTripAndByName(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	st, err := streaming.NewStream("s-general", "general", "all-hands", "w-owner", now, transport.Transport{}, "org-test")
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := s.Streams.Create(ctx, st); err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotByID, err := s.Streams.Get(ctx, "org-test", "s-general")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotByID.Name != "general" {
		t.Fatalf("name = %q", gotByID.Name)
	}
	if gotByID.Transport.Kind != transport.KindLocal {
		t.Fatalf("Transport.Kind = %q, want %q", gotByID.Transport.Kind, transport.KindLocal)
	}
}

func TestSubscriptionsUniquePositionStream(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	sub, _ := streaming.NewSubscription("p-1", "s-1", now, "org-test")
	if err := s.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup, _ := streaming.NewSubscription("p-1", "s-1", now, "org-test")
	if err := s.Subscriptions.Create(ctx, dup); err == nil {
		t.Fatalf("Create duplicate (position,stream) should fail")
	}

	found, err := s.Subscriptions.Find(ctx, "org-test", "p-1", "s-1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.WorkerID != "p-1" || found.StreamID != "s-1" {
		t.Fatalf("subscription = %+v", found)
	}

	if err := s.Subscriptions.Delete(ctx, "org-test", "p-1", "s-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Subscriptions.Find(ctx, "org-test", "p-1", "s-1")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Find after delete: %v, want ErrNotFound", err)
	}
}

func TestEventsListForWorkerViaSubscriptions(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	// Subscriptions are worker-anchored. Seed w-1 and subscribe
	// w-1 → s-a; ListForWorker(w-1) joins events on subscribed
	// streams.
	role, _ := orgchart.NewRole("r-test", "# Test", nil, nil, base, "org-test")
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("Create role: %v", err)
	}
	worker, err := orgchart.NewAIWorker("w-1", "r-test", nil, "# w-1", "org-test")
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := s.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	sub, _ := streaming.NewSubscription("w-1", "s-a", base, "org-test")
	if err := s.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("Create subscription: %v", err)
	}

	e1, _ := streaming.NewEvent("e-1", "s-a", "w-owner", "hello on a", base.Add(time.Second), "org-test")
	e2, _ := streaming.NewEvent("e-2", "s-b", "w-owner", "hello on b", base.Add(2*time.Second), "org-test")
	e3, _ := streaming.NewEvent("e-3", "s-a", "w-owner", "hello again on a", base.Add(3*time.Second), "org-test")
	for _, e := range []streaming.Event{e1, e2, e3} {
		if err := s.Events.Append(ctx, e); err != nil {
			t.Fatalf("Append %s: %v", e.ID, err)
		}
	}

	got, err := s.Events.ListForWorker(ctx, "org-test", "w-1", 0)
	if err != nil {
		t.Fatalf("ListForWorker: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (only s-a visible)", len(got))
	}
	if got[0].ID != "e-3" || got[1].ID != "e-1" {
		t.Fatalf("order wrong: %v", []streaming.EventID{got[0].ID, got[1].ID})
	}

	limited, err := s.Events.ListForWorker(ctx, "org-test", "w-1", 1)
	if err != nil {
		t.Fatalf("ListForWorker limit: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "e-3" {
		t.Fatalf("limit result = %v", limited)
	}
}

func TestEventsListSinceAcrossStreams(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	// Three streams, four events, interleaved across s-a and s-b plus
	// one on s-other (which the caller will exclude).
	for _, e := range []struct {
		id, st, body string
		offset       time.Duration
	}{
		{"e-1", "s-a", "first on a", 1 * time.Second},
		{"e-2", "s-b", "first on b", 2 * time.Second},
		{"e-3", "s-other", "noise", 3 * time.Second},
		{"e-4", "s-a", "second on a", 4 * time.Second},
		{"e-5", "s-b", "second on b", 5 * time.Second},
	} {
		ev, _ := streaming.NewEvent(streaming.EventID(e.id), streaming.StreamID(e.st), "w-owner", e.body, base.Add(e.offset), "org-test")
		if err := s.Events.Append(ctx, ev); err != nil {
			t.Fatalf("Append %s: %v", e.id, err)
		}
	}

	// since="" returns all matching events oldest-first.
	all, err := s.Events.ListSince(ctx, "org-test", []streaming.StreamID{"s-a", "s-b"}, "", 0)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	gotIDs := make([]streaming.EventID, len(all))
	for i, e := range all {
		gotIDs[i] = e.ID
	}
	wantIDs := []streaming.EventID{"e-1", "e-2", "e-4", "e-5"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("ids = %v, want %v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("ids = %v, want %v", gotIDs, wantIDs)
		}
	}

	// since=e-2 returns only events strictly newer than e-2 on the
	// matching streams.
	tail, err := s.Events.ListSince(ctx, "org-test", []streaming.StreamID{"s-a", "s-b"}, "e-2", 0)
	if err != nil {
		t.Fatalf("ListSince since: %v", err)
	}
	if len(tail) != 2 || tail[0].ID != "e-4" || tail[1].ID != "e-5" {
		t.Fatalf("since=e-2 result = %v", tail)
	}

	// Empty stream set returns nothing.
	empty, err := s.Events.ListSince(ctx, "org-test", nil, "", 0)
	if err != nil {
		t.Fatalf("ListSince empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no events, got %v", empty)
	}

	// Unknown since falls through to "no lower bound".
	full, err := s.Events.ListSince(ctx, "org-test", []streaming.StreamID{"s-a"}, "e-stale", 0)
	if err != nil {
		t.Fatalf("ListSince stale: %v", err)
	}
	if len(full) != 2 {
		t.Fatalf("stale-since dropped events: %v", full)
	}
}
