package domain

import (
	"errors"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// Worker is the common abstraction over humans and AI agents occupying
// a Position. HumanWorker and AIWorker are the only concrete
// implementations; the unexported marker method keeps the set closed.
//
// Each Worker holds exactly one Position. A person who serves two
// roles is two Workers — the org graph is the source of truth for who
// is who, and conflating roles into a single Worker would force every
// authz check (which is keyed on workerID + toolName) to additionally
// disambiguate by Position. Singular is the simpler model.
//
// IdentityContent is the per-Worker Identity description (the
// canonical term per ADR-0001 §4 — replaces the earlier "persona /
// profile / candidate" usage). It lives in the domain — never on disk
// — so it survives any change in env layout. Spawners project it
// into the runtime's `identity.md` at activation time.
//
// Domain.Worker carries no runtime-backend state (Helix project IDs,
// session pointers, etc.). That state lives in the WorkerRuntimeState
// sidecar store, keyed by (workerID, backend).
type Worker interface {
	ID() worker.ID
	Kind() worker.Kind
	Position() position.ID
	IdentityContent() string
	// OrganizationID returns the helix.Organization the Worker
	// belongs to. Always non-empty: workers are constructed via
	// NewHumanWorker / NewAIWorker which require it, and the store's
	// composite (id, org_id) PK enforces it at persistence.
	OrganizationID() string
	WithIdentityContent(content string) Worker
	isWorker()
}

// HumanWorker represents a real person inside the organisation.
type HumanWorker struct {
	id              worker.ID
	position        position.ID
	identityContent string
	orgID           string
}

// NewHumanWorker validates and constructs a HumanWorker. Empty
// position is permitted — it represents an archived / vacated
// Worker; tools that hire must pass a non-empty Position. orgID is
// required: every Worker is scoped to a helix.Organization via the
// composite (id, org_id) PK.
func NewHumanWorker(id worker.ID, pos position.ID, identityContent, orgID string) (*HumanWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	if orgID == "" {
		return nil, errors.New("worker orgID is empty")
	}
	return &HumanWorker{id: id, position: pos, identityContent: identityContent, orgID: orgID}, nil
}

func (h *HumanWorker) ID() worker.ID           { return h.id }
func (h *HumanWorker) Kind() worker.Kind       { return worker.KindHuman }
func (h *HumanWorker) Position() position.ID   { return h.position }
func (h *HumanWorker) IdentityContent() string { return h.identityContent }
func (h *HumanWorker) OrganizationID() string  { return h.orgID }
func (h *HumanWorker) WithIdentityContent(content string) Worker {
	return &HumanWorker{id: h.id, position: h.position, identityContent: content, orgID: h.orgID}
}
func (h *HumanWorker) isWorker() {}

// AIWorker represents a software agent inside the organisation.
type AIWorker struct {
	id              worker.ID
	position        position.ID
	identityContent string
	orgID           string
}

// NewAIWorker validates and constructs an AIWorker. orgID is required.
func NewAIWorker(id worker.ID, pos position.ID, identityContent, orgID string) (*AIWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	if orgID == "" {
		return nil, errors.New("worker orgID is empty")
	}
	return &AIWorker{id: id, position: pos, identityContent: identityContent, orgID: orgID}, nil
}

func (a *AIWorker) ID() worker.ID           { return a.id }
func (a *AIWorker) Kind() worker.Kind       { return worker.KindAI }
func (a *AIWorker) Position() position.ID   { return a.position }
func (a *AIWorker) IdentityContent() string { return a.identityContent }
func (a *AIWorker) OrganizationID() string  { return a.orgID }
func (a *AIWorker) WithIdentityContent(content string) Worker {
	return &AIWorker{id: a.id, position: a.position, identityContent: content, orgID: a.orgID}
}
func (a *AIWorker) isWorker() {}
