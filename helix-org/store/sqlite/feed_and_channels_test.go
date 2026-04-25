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
