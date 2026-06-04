package orgchart

import (
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// ToolGrant records that a Worker holds a Tool. The only
// authorisation primitive is `(WorkerID, tool.Name)` — per ADR-0001
// §3 there is no `Scope` field. If a Worker should only be able to
// hire a CFO, that's a CFO-specific tool or a role-prompt constraint,
// not a per-grant rule the runtime enforces.
type ToolGrant struct {
	ID             GrantID
	OrganizationID string
	WorkerID       WorkerID
	ToolName       tool.Name
}

// NewToolGrant validates and constructs a ToolGrant. orgID is required.
func NewToolGrant(id GrantID, workerID WorkerID, toolName tool.Name, orgID string) (ToolGrant, error) {
	if id == "" {
		return ToolGrant{}, errors.New("grant id is empty")
	}
	if workerID == "" {
		return ToolGrant{}, errors.New("grant worker id is empty")
	}
	if toolName == "" {
		return ToolGrant{}, errors.New("grant tool name is empty")
	}
	if orgID == "" {
		return ToolGrant{}, errors.New("grant orgID is empty")
	}
	return ToolGrant{ID: id, OrganizationID: orgID, WorkerID: workerID, ToolName: toolName}, nil
}
