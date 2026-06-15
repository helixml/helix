package transcript

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

type fakeNotifier struct{ calls []streaming.StreamID }

func (f *fakeNotifier) Notify(_ string, sid streaming.StreamID) { f.calls = append(f.calls, sid) }

func fixedNow() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

// TestRecord_AppendsToTranscriptAndNotifies: a turn lands on the Worker's
// s-transcript-<id> stream and wakes observers exactly once.
func TestRecord_AppendsToTranscriptAndNotifies(t *testing.T) {
	t.Parallel()
	st := memory.New()
	notif := &fakeNotifier{}
	rec := New(Deps{Events: st.Events, Notifier: notif, Now: fixedNow, NewID: func() string { return "1" }})

	id, err := rec.Record(context.Background(), "org-test", "w-bob", "hello")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty event id")
	}
	streamID := activation.TranscriptID("w-bob")
	evs, err := st.Events.ListForStream(context.Background(), "org-test", streamID, 10)
	if err != nil {
		t.Fatalf("ListForStream: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("want exactly 1 event on %s, got %d", streamID, len(evs))
	}
	if len(notif.calls) != 1 || notif.calls[0] != streamID {
		t.Fatalf("notify calls = %v, want [%s]", notif.calls, streamID)
	}
}

// TestRecord_BlankBodyIsNoOp: a whitespace-only turn writes nothing — the
// recorder never appends an empty transcript line.
func TestRecord_BlankBodyIsNoOp(t *testing.T) {
	t.Parallel()
	st := memory.New()
	rec := New(Deps{Events: st.Events, Now: fixedNow, NewID: func() string { return "1" }})

	id, err := rec.Record(context.Background(), "org-test", "w-bob", "   ")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if id != "" {
		t.Fatalf("blank body must no-op, got id %q", id)
	}
}

// TestRecord_UnwiredIsNoOp: with no Events repo (the "not wired" case),
// Record short-circuits rather than panicking — matching the previous
// inline behaviour when the org-graph deps were absent.
func TestRecord_UnwiredIsNoOp(t *testing.T) {
	t.Parallel()
	rec := New(Deps{NewID: func() string { return "1" }, Now: fixedNow}) // no Events

	id, err := rec.Record(context.Background(), "org-test", "w-bob", "hello")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if id != "" {
		t.Fatalf("unwired recorder must no-op, got id %q", id)
	}
}
