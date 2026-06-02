package activation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// ID is the typed identifier for one Activation row. Format
// convention: `a-<uuid>` — mirrors `e-…` for events, `w-…` for
// Workers, `s-…` for Streams. Empty IDs are rejected at construction
// (see New).
type ID string

// Activation is the aggregate root for one Spawner invocation. It
// carries:
//
//   - WorkerID: who got woken up.
//   - Triggers: why (one or more — the dispatcher coalesces bursts).
//   - StartedAt / EndedAt: wall-clock bounds of the activation. A nil
//     EndedAt means the activation is still in flight; non-nil means
//     Outcome has been recorded.
//   - Outcome: the terminal state (see outcome.go). Zero until
//     Complete fires.
//   - TranscriptStreamID: deterministic from WorkerID via StreamID().
//     Stored on the row so audit consumers don't have to recompute.
//
// Invariants enforced at construction:
//
//   - ID is non-empty.
//   - WorkerID is non-empty.
//   - len(Triggers) ≥ 1.
//   - TranscriptStreamID == StreamID(WorkerID) — the constructor
//     derives it, callers don't pass it.
//
// Invariants enforced on Complete:
//
//   - EndedAt is after StartedAt.
//   - Outcome is set exactly once. A repeat Complete with the *same*
//     outcome is idempotent (helps reconciliation paths); a repeat
//     with a *different* outcome is an error.
type Activation struct {
	ID                 ID
	OrganizationID     string
	WorkerID           worker.ID
	Triggers           []Trigger
	StartedAt          time.Time
	EndedAt            *time.Time
	Outcome            Outcome
	TranscriptStreamID stream.ID
}

// New constructs an Activation, validating invariants. orgID is
// required. The TranscriptStreamID is derived from WorkerID — callers
// cannot override.
//
// Triggers is copied defensively so subsequent mutation of the
// caller's slice cannot mutate the aggregate.
func New(id ID, workerID worker.ID, triggers []Trigger, startedAt time.Time, orgID string) (*Activation, error) {
	if id == "" {
		return nil, errors.New("activation: ID is empty")
	}
	if workerID == "" {
		return nil, errors.New("activation: WorkerID is empty")
	}
	if len(triggers) == 0 {
		return nil, errors.New("activation: needs at least one trigger")
	}
	if startedAt.IsZero() {
		return nil, errors.New("activation: StartedAt is zero")
	}
	if orgID == "" {
		return nil, errors.New("activation: orgID is empty")
	}
	copied := make([]Trigger, len(triggers))
	copy(copied, triggers)
	return &Activation{
		ID:                 id,
		OrganizationID:     orgID,
		WorkerID:           workerID,
		Triggers:           copied,
		StartedAt:          startedAt,
		TranscriptStreamID: StreamID(workerID),
	}, nil
}

// Complete records the terminal state. Returns an error if endedAt
// is before StartedAt, or if this activation already has a different
// outcome recorded (idempotent on identical re-application).
func (a *Activation) Complete(outcome Outcome, endedAt time.Time) error {
	if endedAt.Before(a.StartedAt) {
		return fmt.Errorf("activation %s: endedAt %v is before startedAt %v", a.ID, endedAt, a.StartedAt)
	}
	if a.EndedAt != nil {
		if a.Outcome == outcome && a.EndedAt.Equal(endedAt) {
			return nil
		}
		return fmt.Errorf("activation %s: already completed with outcome %+v at %v", a.ID, a.Outcome, *a.EndedAt)
	}
	a.EndedAt = &endedAt
	a.Outcome = outcome
	return nil
}

// IsCompleted reports whether Complete has fired on this activation.
// Equivalent to `a.EndedAt != nil`, exposed as a method for read-side
// readability.
func (a *Activation) IsCompleted() bool {
	return a.EndedAt != nil
}

// Repository is the storage port. Implementations live next to the
// host's store package (api/pkg/org/store/gorm — dialect-portable
// GORM, wired against helix's Postgres connection). The interface
// stays in the domain package so the aggregate's persistence
// boundary is explicit and swappable.
//
// Create persists a fresh activation (StartedAt set, EndedAt nil,
// Outcome zero). Complete marks an existing row finished; it is
// keyed by ID so the Spawner can record completion without having
// the original *Activation in hand (e.g. after a process restart
// that finds an orphaned in-flight row).
//
// Get returns the single activation by ID; ListForWorker returns the
// newest-first slice for a Worker, capped at limit (0 or negative ⇒
// implementation's default).
type Repository interface {
	Create(ctx context.Context, a *Activation) error
	Complete(ctx context.Context, orgID string, id ID, outcome Outcome, endedAt time.Time) error
	Get(ctx context.Context, orgID string, id ID) (*Activation, error)
	ListForWorker(ctx context.Context, orgID string, workerID worker.ID, limit int) ([]*Activation, error)
}
