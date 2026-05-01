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

// WorkerKindValues lists every valid WorkerKind. Source of truth for
// the JSON Schema `enum` constraint surfaced through MCP and for
// listing valid options in validation errors. Adding a new kind means
// touching this one place.
func WorkerKindValues() []WorkerKind {
	return []WorkerKind{WorkerKindHuman, WorkerKindAI}
}

// Validate returns an error if k is not one of the known worker kinds.
// The error lists the valid options verbatim so a client that posted
// a bad value can self-correct without reading source.
func (k WorkerKind) Validate() error {
	for _, v := range WorkerKindValues() {
		if k == v {
			return nil
		}
	}
	return fmt.Errorf("unknown worker kind %q (valid: %s)", k, QuotedList(WorkerKindValues()))
}

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
	// HelixSessionID is the ID of the live Helix chat session this Worker
	// is currently bound to, or "" if no session is live. Set by the
	// helixSpawner; never exported through MCP. Empty for human Workers.
	HelixSessionID() string
	WithHelixSessionID(id string) Worker
	// HelixProjectID is the per-Worker Helix project. Created by the
	// spawner on hire; activations open chat sessions against it.
	HelixProjectID() string
	// HelixAgentAppID is the project's auto-provisioned Agent App.
	// Carries `assistants[0].mcps[]` for tool wiring.
	HelixAgentAppID() string
	// HelixRepoID is the project's primary git repo. helix-specs
	// branch + job/* path holds the Worker's role/identity/agent.md.
	HelixRepoID() string
	WithHelixProject(projectID, agentAppID, repoID string) Worker
	isWorker()
}

// HumanWorker represents a real person inside the organisation.
type HumanWorker struct {
	id              WorkerID
	positions       []PositionID
	identity        string
	helixSessionID  string
	helixProjectID  string
	helixAgentAppID string
	helixRepoID     string
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
func (h *HumanWorker) HelixSessionID() string  { return h.helixSessionID }
func (h *HumanWorker) HelixProjectID() string  { return h.helixProjectID }
func (h *HumanWorker) HelixAgentAppID() string { return h.helixAgentAppID }
func (h *HumanWorker) HelixRepoID() string     { return h.helixRepoID }
func (h *HumanWorker) WithIdentityContent(content string) Worker {
	return &HumanWorker{id: h.id, positions: copyPositions(h.positions), identity: content, helixSessionID: h.helixSessionID, helixProjectID: h.helixProjectID, helixAgentAppID: h.helixAgentAppID, helixRepoID: h.helixRepoID}
}
func (h *HumanWorker) WithHelixSessionID(id string) Worker {
	return &HumanWorker{id: h.id, positions: copyPositions(h.positions), identity: h.identity, helixSessionID: id, helixProjectID: h.helixProjectID, helixAgentAppID: h.helixAgentAppID, helixRepoID: h.helixRepoID}
}
func (h *HumanWorker) WithHelixProject(projectID, agentAppID, repoID string) Worker {
	return &HumanWorker{id: h.id, positions: copyPositions(h.positions), identity: h.identity, helixSessionID: h.helixSessionID, helixProjectID: projectID, helixAgentAppID: agentAppID, helixRepoID: repoID}
}
func (h *HumanWorker) isWorker() {}

// AIWorker represents a software agent inside the organisation.
type AIWorker struct {
	id              WorkerID
	positions       []PositionID
	identity        string
	helixSessionID  string
	helixProjectID  string
	helixAgentAppID string
	helixRepoID     string
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
func (a *AIWorker) HelixSessionID() string  { return a.helixSessionID }
func (a *AIWorker) HelixProjectID() string  { return a.helixProjectID }
func (a *AIWorker) HelixAgentAppID() string { return a.helixAgentAppID }
func (a *AIWorker) HelixRepoID() string     { return a.helixRepoID }
func (a *AIWorker) WithIdentityContent(content string) Worker {
	return &AIWorker{id: a.id, positions: copyPositions(a.positions), identity: content, helixSessionID: a.helixSessionID, helixProjectID: a.helixProjectID, helixAgentAppID: a.helixAgentAppID, helixRepoID: a.helixRepoID}
}
func (a *AIWorker) WithHelixSessionID(id string) Worker {
	return &AIWorker{id: a.id, positions: copyPositions(a.positions), identity: a.identity, helixSessionID: id, helixProjectID: a.helixProjectID, helixAgentAppID: a.helixAgentAppID, helixRepoID: a.helixRepoID}
}
func (a *AIWorker) WithHelixProject(projectID, agentAppID, repoID string) Worker {
	return &AIWorker{id: a.id, positions: copyPositions(a.positions), identity: a.identity, helixSessionID: a.helixSessionID, helixProjectID: projectID, helixAgentAppID: agentAppID, helixRepoID: repoID}
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
