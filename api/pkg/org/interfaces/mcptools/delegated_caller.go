package mcptools

import (
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// DelegatedCaller wraps a spec task's caller when its project is bound to
// an Agent: tools see the Agent as the caller, while audit keeps naming the
// task. Unbonded tasks are never wrapped.
type DelegatedCaller struct {
	Inner       tool.Caller
	Agent       string
	AuditTaskID string
}

func (d DelegatedCaller) ID() string                         { return d.Agent }
func (d DelegatedCaller) OrganizationID() string             { return d.Inner.OrganizationID() }
func (d DelegatedCaller) AuditActorType() orgaudit.ActorType { return orgaudit.ActorSpecTask }
func (d DelegatedCaller) AuditActorID() string               { return d.AuditTaskID }

var _ tool.Caller = DelegatedCaller{}
