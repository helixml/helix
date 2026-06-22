package gorm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// newActivationStore opens a per-test Postgres-schema Store for
// activation round-trip tests.
func newActivationStore(t *testing.T) *store.Store {
	t.Helper()
	return GetOrgTestDB(t)
}

// TestActivationCreateGetRoundTrip pins the storage seam for the
// happy path: Create persists every field on the aggregate, Get
// reads back an equivalent struct.
func TestActivationCreateGetRoundTrip(t *testing.T) {
	t.Parallel()
	s := newActivationStore(t)
	ctx := context.Background()

	started := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	triggers := []activation.Trigger{
		{Kind: activation.TriggerHire},
		{
			Kind:       activation.TriggerEvent,
			EventID:    "e-1",
			TopicID:   "s-test",
			Source:     "w-bob",
			SourceKind: "ai",
			Message:    streaming.Message{From: "w-bob", Body: "hi"},
			CreatedAt:  started.Add(-time.Minute),
		},
	}
	a, err := activation.New("a-1", "w-alice", triggers, started, "org-test")
	if err != nil {
		t.Fatalf("new activation: %v", err)
	}
	if err := s.Activations.Create(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.Activations.Get(ctx, "org-test", "a-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != a.ID {
		t.Errorf("ID = %q, want %q", got.ID, a.ID)
	}
	if got.WorkerID != a.WorkerID {
		t.Errorf("WorkerID = %q, want %q", got.WorkerID, a.WorkerID)
	}
	if !got.StartedAt.Equal(a.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, a.StartedAt)
	}
	if got.EndedAt != nil {
		t.Errorf("EndedAt = %v, want nil (no Complete yet)", got.EndedAt)
	}
	if got.TranscriptID != activation.TranscriptID("w-alice") {
		t.Errorf("TranscriptID = %q, want derived from WorkerID", got.TranscriptID)
	}
	if len(got.Triggers) != 2 {
		t.Fatalf("len(Triggers) = %d, want 2", len(got.Triggers))
	}
	if got.Triggers[0].Kind != activation.TriggerHire {
		t.Errorf("Triggers[0].Kind = %q, want hire", got.Triggers[0].Kind)
	}
	if got.Triggers[1].EventID != "e-1" {
		t.Errorf("Triggers[1].EventID = %q, want e-1", got.Triggers[1].EventID)
	}
	if got.Triggers[1].Message.Body != "hi" {
		t.Errorf("Triggers[1].Message.Body = %q, want hi (round-tripped through JSON)", got.Triggers[1].Message.Body)
	}
}

// TestActivationCompleteRecordsOutcome confirms Complete persists
// EndedAt + Outcome and a subsequent Get reflects them.
func TestActivationCompleteRecordsOutcome(t *testing.T) {
	t.Parallel()
	s := newActivationStore(t)
	ctx := context.Background()
	started := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	a, _ := activation.New("a-1", "w-alice", []activation.Trigger{{Kind: activation.TriggerHire}}, started, "org-test")
	if err := s.Activations.Create(ctx, a); err != nil {
		t.Fatalf("create: %v", err)
	}
	ended := started.Add(30 * time.Second)
	out := activation.Outcome{Status: activation.StatusError, Error: "boom"}
	if err := s.Activations.Complete(ctx, "org-test", "a-1", out, ended); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := s.Activations.Get(ctx, "org-test", "a-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.EndedAt == nil || !got.EndedAt.Equal(ended) {
		t.Errorf("EndedAt = %v, want %v", got.EndedAt, ended)
	}
	if got.Outcome != out {
		t.Errorf("Outcome = %+v, want %+v", got.Outcome, out)
	}
}

// TestActivationGetUnknownIsNotFound is the standard store contract —
// missing rows surface as ErrNotFound so callers can errors.Is them.
func TestActivationGetUnknownIsNotFound(t *testing.T) {
	t.Parallel()
	s := newActivationStore(t)
	_, err := s.Activations.Get(context.Background(), "org-test", "a-missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get unknown = %v, want errors.Is(_, store.ErrNotFound)", err)
	}
}

// TestActivationCompleteUnknownIsNotFound — Complete on a missing
// row is also an ErrNotFound, not a silent no-op.
func TestActivationCompleteUnknownIsNotFound(t *testing.T) {
	t.Parallel()
	s := newActivationStore(t)
	err := s.Activations.Complete(
		context.Background(),
		"org-test",
		"a-missing",
		activation.Outcome{Status: activation.StatusOK},
		time.Now(),
	)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Complete unknown = %v, want errors.Is(_, store.ErrNotFound)", err)
	}
}

// TestActivationListForWorkerReturnsNewestFirst exercises the audit
// surface: ListForWorker must return rows ordered by StartedAt
// descending, scoped to one Worker, capped at the requested limit.
func TestActivationListForWorkerReturnsNewestFirst(t *testing.T) {
	t.Parallel()
	s := newActivationStore(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)

	create := func(id activation.ID, w string, at time.Time) {
		a, err := activation.New(id, "worker_id_overridden", []activation.Trigger{{Kind: activation.TriggerHire}}, at, "org-test")
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		// Re-set WorkerID directly because the constructor enforces non-empty;
		// keeping the parameterised id avoids per-test boilerplate.
		a.WorkerID = orgchart.WorkerID(w)
		a.TranscriptID = activation.TranscriptID(a.WorkerID)
		if err := s.Activations.Create(ctx, a); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	create("a-1", "w-alice", base)
	create("a-2", "w-alice", base.Add(10*time.Second))
	create("a-3", "w-alice", base.Add(20*time.Second))
	create("a-4", "w-bob", base.Add(15*time.Second))

	rows, err := s.Activations.ListForWorker(ctx, "org-test", "w-alice", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("len = %d, want 3 (scoped to w-alice)", len(rows))
	}
	wantOrder := []activation.ID{"a-3", "a-2", "a-1"}
	for i, r := range rows {
		if r.ID != wantOrder[i] {
			t.Errorf("rows[%d].ID = %q, want %q", i, r.ID, wantOrder[i])
		}
	}

	// Limit clamps the returned slice.
	limited, err := s.Activations.ListForWorker(ctx, "org-test", "w-alice", 2)
	if err != nil {
		t.Fatalf("list limit: %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("len(limit=2) = %d, want 2", len(limited))
	}
}
