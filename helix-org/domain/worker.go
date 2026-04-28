package domain

import (
	"errors"
	"fmt"
)

// WorkerKind distinguishes HumanWorker from AIWorker.
type WorkerKind string

const (
	WorkerKindHuman WorkerKind = "human"
	WorkerKindAI    WorkerKind = "ai"
)

// Worker is the common abstraction over humans and AI agents occupying
// Positions. HumanWorker and AIWorker are the only concrete
// implementations; the unexported marker method keeps the set closed.
//
// IdentityContent is the per-Worker description (persona for AI, profile
// for a human). It lives in the domain — never on disk — so it survives
// any change in env layout (local files today, remote workspaces
// tomorrow). Spawners project it into whatever the env channel needs at
// activation time.
type Worker interface {
	ID() WorkerID
	Kind() WorkerKind
	Positions() []PositionID
	IdentityContent() string
	WithIdentityContent(content string) Worker
	isWorker()
}

// HumanWorker represents a real person inside the organisation.
type HumanWorker struct {
	id        WorkerID
	positions []PositionID
	identity  string
}

// NewHumanWorker validates and constructs a HumanWorker.
func NewHumanWorker(id WorkerID, positions []PositionID, identityContent string) (*HumanWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	ps, err := validatePositions(positions)
	if err != nil {
		return nil, err
	}
	return &HumanWorker{id: id, positions: ps, identity: identityContent}, nil
}

func (h *HumanWorker) ID() WorkerID            { return h.id }
func (h *HumanWorker) Kind() WorkerKind        { return WorkerKindHuman }
func (h *HumanWorker) Positions() []PositionID { return copyPositions(h.positions) }
func (h *HumanWorker) IdentityContent() string { return h.identity }
func (h *HumanWorker) WithIdentityContent(content string) Worker {
	return &HumanWorker{id: h.id, positions: copyPositions(h.positions), identity: content}
}
func (h *HumanWorker) isWorker() {}

// AIWorker represents a software agent inside the organisation.
type AIWorker struct {
	id        WorkerID
	positions []PositionID
	identity  string
}

// NewAIWorker validates and constructs an AIWorker.
func NewAIWorker(id WorkerID, positions []PositionID, identityContent string) (*AIWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	ps, err := validatePositions(positions)
	if err != nil {
		return nil, err
	}
	return &AIWorker{id: id, positions: ps, identity: identityContent}, nil
}

func (a *AIWorker) ID() WorkerID            { return a.id }
func (a *AIWorker) Kind() WorkerKind        { return WorkerKindAI }
func (a *AIWorker) Positions() []PositionID { return copyPositions(a.positions) }
func (a *AIWorker) IdentityContent() string { return a.identity }
func (a *AIWorker) WithIdentityContent(content string) Worker {
	return &AIWorker{id: a.id, positions: copyPositions(a.positions), identity: content}
}
func (a *AIWorker) isWorker() {}

func validatePositions(positions []PositionID) ([]PositionID, error) {
	// Zero positions is permitted: it represents an archived/vacated Worker.
	// Tools that hire must pass >=1.
	seen := make(map[PositionID]struct{}, len(positions))
	out := make([]PositionID, 0, len(positions))
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

func copyPositions(positions []PositionID) []PositionID {
	out := make([]PositionID, len(positions))
	copy(out, positions)
	return out
}
