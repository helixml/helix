package domain

import "errors"

// ToolGrant records that a Worker holds a Tool. The only authorisation
// primitive is `(WorkerID, ToolName)` — per ADR-0001 §3 there is no
// `Scope` field. If a Worker should only be able to hire a CFO, that's
// a CFO-specific tool or a role-prompt constraint, not a per-grant rule
// the runtime enforces.
type ToolGrant struct {
	ID       GrantID
	WorkerID WorkerID
	ToolName ToolName
}

// NewToolGrant validates and constructs a ToolGrant.
func NewToolGrant(id GrantID, workerID WorkerID, toolName ToolName) (ToolGrant, error) {
	if id == "" {
		return ToolGrant{}, errors.New("grant id is empty")
	}
	if workerID == "" {
		return ToolGrant{}, errors.New("grant worker id is empty")
	}
	if toolName == "" {
		return ToolGrant{}, errors.New("grant tool name is empty")
	}
	return ToolGrant{ID: id, WorkerID: workerID, ToolName: toolName}, nil
}
