package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

func TestChannelsRoundTripAndByName(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	ch, err := domain.NewChannel("c-general", "general", "all-hands", "w-owner", now)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if err := s.Channels.Create(ctx, ch); err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotByID, err := s.Channels.Get(ctx, "c-general")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotByID.Name != "general" {
		t.Fatalf("name = %q", gotByID.Name)
	}
}

func TestStreamsUniqueWorkerChannel(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	stream, _ := domain.NewStream("s-1", "w-1", "c-1", now)
	if err := s.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("Create: %v", err)
	}

	dup, _ := domain.NewStream("s-2", "w-1", "c-1", now)
	if err := s.Streams.Create(ctx, dup); err == nil {
		t.Fatalf("Create duplicate (worker,channel) should fail")
	}

	found, err := s.Streams.FindForWorkerAndChannel(ctx, "w-1", "c-1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.ID != "s-1" {
		t.Fatalf("id = %q", found.ID)
	}

	if err := s.Streams.Delete(ctx, "s-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = s.Streams.FindForWorkerAndChannel(ctx, "w-1", "c-1")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Find after delete: %v, want ErrNotFound", err)
	}
}

func TestEventsListForWorkerViaStreams(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	// Two channels, w-1 subscribed only to c-a.
	s1, _ := domain.NewStream("s-1", "w-1", "c-a", base)
	if err := s.Streams.Create(ctx, s1); err != nil {
		t.Fatalf("Create stream: %v", err)
	}

	e1, _ := domain.NewEvent("e-1", "c-a", "w-owner", "hello on a", base.Add(time.Second))
	e2, _ := domain.NewEvent("e-2", "c-b", "w-owner", "hello on b", base.Add(2*time.Second))
	e3, _ := domain.NewEvent("e-3", "c-a", "w-owner", "hello again on a", base.Add(3*time.Second))
	for _, e := range []domain.Event{e1, e2, e3} {
		if err := s.Events.Append(ctx, e); err != nil {
			t.Fatalf("Append %s: %v", e.ID, err)
		}
	}

	got, err := s.Events.ListForWorker(ctx, "w-1", 0)
	if err != nil {
		t.Fatalf("ListForWorker: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (only c-a visible)", len(got))
	}
	if got[0].ID != "e-3" || got[1].ID != "e-1" {
		t.Fatalf("order wrong: %v", []domain.EventID{got[0].ID, got[1].ID})
	}

	limited, err := s.Events.ListForWorker(ctx, "w-1", 1)
	if err != nil {
		t.Fatalf("ListForWorker limit: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "e-3" {
		t.Fatalf("limit result = %v", limited)
	}
}

func TestEventsListSinceAcrossChannels(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)

	// Three channels, four events, interleaved across c-a and c-b plus
	// one on c-other (which the caller will exclude).
	for _, e := range []struct {
		id, ch, body string
		offset       time.Duration
	}{
		{"e-1", "c-a", "first on a", 1 * time.Second},
		{"e-2", "c-b", "first on b", 2 * time.Second},
		{"e-3", "c-other", "noise", 3 * time.Second},
		{"e-4", "c-a", "second on a", 4 * time.Second},
		{"e-5", "c-b", "second on b", 5 * time.Second},
	} {
		ev, _ := domain.NewEvent(domain.EventID(e.id), domain.ChannelID(e.ch), "w-owner", e.body, base.Add(e.offset))
		if err := s.Events.Append(ctx, ev); err != nil {
			t.Fatalf("Append %s: %v", e.id, err)
		}
	}

	// since="" returns all matching events oldest-first.
	all, err := s.Events.ListSince(ctx, []domain.ChannelID{"c-a", "c-b"}, "", 0)
	if err != nil {
		t.Fatalf("ListSince: %v", err)
	}
	gotIDs := make([]domain.EventID, len(all))
	for i, e := range all {
		gotIDs[i] = e.ID
	}
	wantIDs := []domain.EventID{"e-1", "e-2", "e-4", "e-5"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("ids = %v, want %v", gotIDs, wantIDs)
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("ids = %v, want %v", gotIDs, wantIDs)
		}
	}

	// since=e-2 returns only events strictly newer than e-2 on the
	// matching channels.
	tail, err := s.Events.ListSince(ctx, []domain.ChannelID{"c-a", "c-b"}, "e-2", 0)
	if err != nil {
		t.Fatalf("ListSince since: %v", err)
	}
	if len(tail) != 2 || tail[0].ID != "e-4" || tail[1].ID != "e-5" {
		t.Fatalf("since=e-2 result = %v", tail)
	}

	// Empty channel set returns nothing.
	empty, err := s.Events.ListSince(ctx, nil, "", 0)
	if err != nil {
		t.Fatalf("ListSince empty: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no events, got %v", empty)
	}

	// Unknown since falls through to "no lower bound".
	full, err := s.Events.ListSince(ctx, []domain.ChannelID{"c-a"}, "e-stale", 0)
	if err != nil {
		t.Fatalf("ListSince stale: %v", err)
	}
	if len(full) != 2 {
		t.Fatalf("stale-since dropped events: %v", full)
	}
}
