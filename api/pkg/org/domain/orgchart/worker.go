package orgchart

import "errors"

// Worker is the common abstraction over humans and AI agents
// occupying a Position. HumanWorker and AIWorker are the only
// concrete implementations; the unexported marker method keeps the
// set closed.
//
// Each Worker holds exactly one Position. A person who serves two
// roles is two Workers — the org graph is the source of truth for
// who is who.
//
// IdentityContent is the per-Worker Identity description. It lives
// in the domain — never on disk — so it survives any change in env
// layout. Spawners project it into the runtime's `identity.md` at
// activation time.
type Worker interface {
	ID() WorkerID
	Kind() WorkerKind
	Position() PositionID
	IdentityContent() string
	OrganizationID() string
	WithIdentityContent(content string) Worker
	isWorker()
}

// HumanWorker represents a real person inside the organisation.
type HumanWorker struct {
	id              WorkerID
	position        PositionID
	identityContent string
	orgID           string
}

// NewHumanWorker validates and constructs a HumanWorker. Empty
// position is permitted — it represents an archived / vacated
// Worker; tools that hire must pass a non-empty Position. orgID is
// required: every Worker is scoped to a helix.Organization via the
// composite (id, org_id) PK.
func NewHumanWorker(id WorkerID, pos PositionID, identityContent, orgID string) (*HumanWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	if orgID == "" {
		return nil, errors.New("worker orgID is empty")
	}
	return &HumanWorker{id: id, position: pos, identityContent: identityContent, orgID: orgID}, nil
}

func (h *HumanWorker) ID() WorkerID            { return h.id }
func (h *HumanWorker) Kind() WorkerKind        { return WorkerKindHuman }
func (h *HumanWorker) Position() PositionID    { return h.position }
func (h *HumanWorker) IdentityContent() string { return h.identityContent }
func (h *HumanWorker) OrganizationID() string  { return h.orgID }
func (h *HumanWorker) WithIdentityContent(content string) Worker {
	return &HumanWorker{id: h.id, position: h.position, identityContent: content, orgID: h.orgID}
}
func (h *HumanWorker) isWorker() {}

// AIWorker represents a software agent inside the organisation.
type AIWorker struct {
	id              WorkerID
	position        PositionID
	identityContent string
	orgID           string
}

// NewAIWorker validates and constructs an AIWorker. orgID is required.
func NewAIWorker(id WorkerID, pos PositionID, identityContent, orgID string) (*AIWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	if orgID == "" {
		return nil, errors.New("worker orgID is empty")
	}
	return &AIWorker{id: id, position: pos, identityContent: identityContent, orgID: orgID}, nil
}

func (a *AIWorker) ID() WorkerID            { return a.id }
func (a *AIWorker) Kind() WorkerKind        { return WorkerKindAI }
func (a *AIWorker) Position() PositionID    { return a.position }
func (a *AIWorker) IdentityContent() string { return a.identityContent }
func (a *AIWorker) OrganizationID() string  { return a.orgID }
func (a *AIWorker) WithIdentityContent(content string) Worker {
	return &AIWorker{id: a.id, position: a.position, identityContent: content, orgID: a.orgID}
}
func (a *AIWorker) isWorker() {}
