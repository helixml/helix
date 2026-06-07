package orgchart

import "errors"

// Worker is the common abstraction over humans and AI agents in the
// organisation. HumanWorker and AIWorker are the only concrete
// implementations; the unexported marker method keeps the set closed.
//
// Each Worker carries a Role (the capability binding — the live source
// of truth for the Worker's MCP surface). Reporting lines are a
// separate many-to-many relation (see ReportingLine) rather than a
// field on the Worker, so a Worker can report to several managers. A
// person who serves two roles is two Workers.
//
// IdentityContent is the per-Worker Identity description. It lives
// in the domain — never on disk — so it survives any change in env
// layout. Spawners project it into the runtime's `identity.md` at
// activation time.
type Worker interface {
	ID() WorkerID
	Kind() WorkerKind
	RoleID() RoleID
	IdentityContent() string
	OrganizationID() string
	WithIdentityContent(content string) Worker
	isWorker()
}

// HumanWorker represents a real person inside the organisation.
type HumanWorker struct {
	id              WorkerID
	roleID          RoleID
	identityContent string
	orgID           string
}

// NewHumanWorker validates and constructs a HumanWorker. id, roleID
// and orgID are required.
func NewHumanWorker(id WorkerID, roleID RoleID, identityContent, orgID string) (*HumanWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	if roleID == "" {
		return nil, errors.New("worker roleID is empty")
	}
	if orgID == "" {
		return nil, errors.New("worker orgID is empty")
	}
	return &HumanWorker{id: id, roleID: roleID, identityContent: identityContent, orgID: orgID}, nil
}

func (h *HumanWorker) ID() WorkerID            { return h.id }
func (h *HumanWorker) Kind() WorkerKind        { return WorkerKindHuman }
func (h *HumanWorker) RoleID() RoleID          { return h.roleID }
func (h *HumanWorker) IdentityContent() string { return h.identityContent }
func (h *HumanWorker) OrganizationID() string  { return h.orgID }
func (h *HumanWorker) WithIdentityContent(content string) Worker {
	return &HumanWorker{id: h.id, roleID: h.roleID, identityContent: content, orgID: h.orgID}
}
func (h *HumanWorker) isWorker() {}

// AIWorker represents a software agent inside the organisation.
type AIWorker struct {
	id              WorkerID
	roleID          RoleID
	identityContent string
	orgID           string
}

// NewAIWorker validates and constructs an AIWorker.
func NewAIWorker(id WorkerID, roleID RoleID, identityContent, orgID string) (*AIWorker, error) {
	if id == "" {
		return nil, errors.New("worker id is empty")
	}
	if roleID == "" {
		return nil, errors.New("worker roleID is empty")
	}
	if orgID == "" {
		return nil, errors.New("worker orgID is empty")
	}
	return &AIWorker{id: id, roleID: roleID, identityContent: identityContent, orgID: orgID}, nil
}

func (a *AIWorker) ID() WorkerID            { return a.id }
func (a *AIWorker) Kind() WorkerKind        { return WorkerKindAI }
func (a *AIWorker) RoleID() RoleID          { return a.roleID }
func (a *AIWorker) IdentityContent() string { return a.identityContent }
func (a *AIWorker) OrganizationID() string  { return a.orgID }
func (a *AIWorker) WithIdentityContent(content string) Worker {
	return &AIWorker{id: a.id, roleID: a.roleID, identityContent: content, orgID: a.orgID}
}
func (a *AIWorker) isWorker() {}
