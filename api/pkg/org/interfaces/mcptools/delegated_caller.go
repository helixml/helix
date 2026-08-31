package mcptools

import (
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// DelegatedCaller is the identity a spec task presents to org tools when
// its project is bound to an Agent: every tool sees exactly one acting
// identity — the Agent — because at the trust boundary there is exactly
// one party whose authority is in play. Resolving the identity once here
// (instead of per tool) is what lets the tool implementations stay
// written for Bots with no concept of delegation; the blast radius of a
// delegated call can never exceed "what that Agent could do", and the
// served tool list (mcptools.SpecTaskBlockedTools, minus the org-admin
// names) is what keeps delegation from becoming org-graph admin.
//
// A task with no bond is never wrapped: it is served only the catalogue's
// spec-task CRUD tools, whose semantics are genuinely keyed on the task
// itself.
//
// Audit is deliberately NOT the acting identity: audit answers "who
// pulled the trigger", so AuditActorID names the task and
// AuditActorType labels it spec_task (see orgaudit.SelfDescribingActorID).
// Row-writing attribution that names a creator follows the same rule and
// the tools already attribute via the ProjectPrincipal on ctx.
type DelegatedCaller struct {
	Inner tool.Caller
	// Agent is the bound Agent's node id — the identity tools act as.
	Agent string
	// AuditTaskID is the spec task id the audit log attributes to.
	AuditTaskID string
}

func (d DelegatedCaller) ID() string                         { return d.Agent }
func (d DelegatedCaller) OrganizationID() string             { return d.Inner.OrganizationID() }
func (d DelegatedCaller) AuditActorType() orgaudit.ActorType { return orgaudit.ActorSpecTask }
func (d DelegatedCaller) AuditActorID() string               { return d.AuditTaskID }

var _ tool.Caller = DelegatedCaller{}
