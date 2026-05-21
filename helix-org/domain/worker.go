package domain

import (
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// Worker is the common abstraction over humans and AI agents occupying
// Positions. HumanWorker and AIWorker are the only concrete
// implementations; the unexported marker method keeps the set closed.
//
// IdentityContent is the per-Worker Identity description (the
// canonical term per ADR-0001 §4 — replaces the earlier "persona /
// profile / candidate" usage). It lives in the domain — never on disk
// — so it survives any change in env layout (local files today, remote
// workspaces tomorrow). Spawners project it into the runtime's
// `identity.md` at activation time.
//
// Domain.Worker carries no runtime-backend state (Helix project IDs,
// session pointers, etc.). That state lives in the WorkerRuntimeState
// sidecar store, keyed by (workerID, backend) — added without touching
// the domain when a new runtime backend appears.
type Worker interface {
	ID() worker.ID
	Kind() worker.Kind
	Positions() []position.ID
	IdentityContent() string
	WithIdentityContent(content string) Worker
	isWorker()
}

// HumanWorker represents a real person inside the organisation.
type HumanWorker struct {
	id        worker.ID
	positions []position.ID
	identity  string
}

// NewHumanWorker validates and constructs a HumanWorker.
func NewHumanWorker(id worker.ID, positions []position.ID, identityContent string) (*HumanWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	ps, err := validatePositions(positions)
	if err != nil {
		return nil, err
	}
	return &HumanWorker{id: id, positions: ps, identity: identityContent}, nil
}

func (h *HumanWorker) ID() worker.ID            { return h.id }
func (h *HumanWorker) Kind() worker.Kind         { return worker.KindHuman }
func (h *HumanWorker) Positions() []position.ID { return copyPositions(h.positions) }
func (h *HumanWorker) IdentityContent() string  { return h.identity }
func (h *HumanWorker) WithIdentityContent(content string) Worker {
	return &HumanWorker{id: h.id, positions: copyPositions(h.positions), identity: content}
}
func (h *HumanWorker) isWorker() {}

// AIWorker represents a software agent inside the organisation.
type AIWorker struct {
	id        worker.ID
	positions []position.ID
	identity  string
}

// NewAIWorker validates and constructs an AIWorker.
func NewAIWorker(id worker.ID, positions []position.ID, identityContent string) (*AIWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	ps, err := validatePositions(positions)
	if err != nil {
		return nil, err
	}
	return &AIWorker{id: id, positions: ps, identity: identityContent}, nil
}

func (a *AIWorker) ID() worker.ID            { return a.id }
func (a *AIWorker) Kind() worker.Kind         { return worker.KindAI }
func (a *AIWorker) Positions() []position.ID { return copyPositions(a.positions) }
func (a *AIWorker) IdentityContent() string  { return a.identity }
func (a *AIWorker) WithIdentityContent(content string) Worker {
	return &AIWorker{id: a.id, positions: copyPositions(a.positions), identity: content}
}
func (a *AIWorker) isWorker() {}

func validatePositions(positions []position.ID) ([]position.ID, error) {
	// Zero positions is permitted: it represents an archived/vacated Worker.
	// Tools that hire must pass >=1.
	seen := make(map[position.ID]struct{}, len(positions))
	out := make([]position.ID, 0, len(positions))
	for _, p := range positions {
		if p == "" {
			return nil, errors.New("position id is empty")
		}
		if _, dup := seen[p]; dup {
			return nil, fmt.Errorf("duplicate position %q", p)
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

func copyPositions(positions []position.ID) []position.ID {
	out := make([]position.ID, len(positions))
	copy(out, positions)
	return out
}
