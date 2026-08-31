package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/types"
)

// SpecTaskMCPBackend gives a spec task's coding agent a slice of the same MCP
// tool registry org Bots use, so a task can create and steer other tasks as
// sub-agents.
//
// The agent authenticates with the session-scoped api key its sandbox already
// holds; identity comes only from the stored session named by that key, never
// from the URL. Which tools it sees is the union of the project's allowlist and
// the task's own extras, intersected with the spec-task-eligible catalogue.
type SpecTaskMCPBackend struct {
	apiServer *HelixAPIServer
	mcpServer *helixorgserver.Server
	scope     *helixOrgScope
}

// NewSpecTaskMCPBackend creates the backend over the in-process org MCP server.
func NewSpecTaskMCPBackend(apiServer *HelixAPIServer, orgHandlers *helixOrgHandlers) *SpecTaskMCPBackend {
	return &SpecTaskMCPBackend{
		apiServer: apiServer,
		mcpServer: orgHandlers.mcpServer,
		scope:     orgHandlers.scope,
	}
}

// specTaskCaller is the tool.Caller for a spec task acting on its own behalf.
// The task ID is the actor id, so audit entries attribute the call to the task
// rather than to a Bot that does not exist.
type specTaskCaller struct{ id, orgID string }

func (c specTaskCaller) ID() string             { return c.id }
func (c specTaskCaller) OrganizationID() string { return c.orgID }

// AuditActorType keeps the org audit log honest: this caller is a spec task,
// not a Bot.
func (c specTaskCaller) AuditActorType() orgaudit.ActorType { return orgaudit.ActorSpecTask }

// ServeHTTP implements MCPBackend. The gateway has already authenticated the
// api key; everything below re-derives authority from the session it names.
func (b *SpecTaskMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	if suffix := strings.Trim(mux.Vars(r)["path"], "/"); suffix != "" {
		http.Error(w, "spec-task MCP does not accept a suffix path", http.StatusBadRequest)
		return
	}
	if user == nil || user.TokenType != types.TokenTypeAPIKey || user.SessionID == "" {
		http.Error(w, "session-scoped API key required for spec-task MCP access", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	session, err := b.apiServer.Store.GetSession(ctx, user.SessionID)
	if err != nil || session == nil || session.Owner != user.ID || session.Metadata.SpecTaskID == "" {
		http.Error(w, "session is not authorized for spec-task MCP access", http.StatusForbidden)
		return
	}
	task, err := b.apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil {
		http.Error(w, "spec task not found", http.StatusForbidden)
		return
	}
	project, err := b.apiServer.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		http.Error(w, "project not found", http.StatusForbidden)
		return
	}
	if project.OrganizationID == "" {
		http.Error(w, "spec-task MCP is only available to organization projects", http.StatusForbidden)
		return
	}
	if _, err := b.apiServer.authorizeOrgMember(ctx, user, project.OrganizationID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	tools, bound := b.apiServer.specTaskToolSurface(ctx, project, task)
	if len(tools) == 0 {
		http.Error(w, "no Helix tools are enabled for this task", http.StatusForbidden)
		return
	}
	if err := b.scope.ensureBootstrap(ctx, project.OrganizationID); err != nil {
		http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Trace().
		Str("user_id", user.ID).
		Str("spec_task_id", task.ID).
		Str("project_id", project.ID).
		Int("tools", len(tools)).
		Msg("spec-task MCP: serving project-scoped caller")

	// The project and the user the task's mutations are attributed to ride the
	// context; the org runtime reads them instead of per-Worker state.
	ctx = helixorgserver.WithOrgID(ctx, project.OrganizationID)
	ctx = runtime.WithProjectPrincipal(ctx, runtime.ProjectPrincipal{
		ProjectID:    project.ID,
		ActingUserID: task.UserID,
	})
	// The Agent whose project the task lives in (if any) rides the context:
	// tools act on behalf of it (see mcptools.SubjectForCaller) while audit
	// attribution stays on the task caller.
	if bound != "" {
		ctx = runtime.WithBoundWorker(ctx, bound)
	}
	caller := specTaskCaller{id: task.ID, orgID: project.OrganizationID}
	b.mcpServer.ServeMCPForCaller(w, r.WithContext(ctx), caller, tools)
}

// specTaskToolSurface composes the full org tool surface a task receives:
// its catalogued own grants (project allowlist ∪ task extras) UNION the live
// tool surface of the Agent whose runtime home project is the task's project
// (empty + zero bond when none — today's behaviour exactly). Both the MCP
// gate here and the sandbox-config rev (specTaskAgentTools) must call this
// ONE function: tools/list, the rev cache-bust, and the REST view can only
// agree if they share one code path. The returned bond is what the backend
// stashes for SubjectForCaller.
func (s *HelixAPIServer) specTaskToolSurface(ctx context.Context, project *types.Project, task *types.SpecTask) ([]tool.Name, orgchart.NodeID) {
	own := eligibleSpecTaskTools(project, task)
	if s.helixOrg == nil || s.helixOrg.store == nil || project.OrganizationID == "" {
		return own, ""
	}
	bound, err := runtimehelix.BoundAgentForProject(ctx, s.helixOrg.store, project.OrganizationID, project.ID)
	if err != nil {
		return own, ""
	}
	seen := make(map[string]struct{}, len(own))
	for _, name := range own {
		seen[string(name)] = struct{}{}
	}
	merged := own
	for _, name := range runtimehelix.AgentToolNames(ctx, s.helixOrg.store, project.OrganizationID, bound) {
		if _, dup := seen[string(name)]; dup {
			continue
		}
		seen[string(name)] = struct{}{}
		merged = append(merged, name)
	}
	return merged, bound
}

// eligibleSpecTaskTools is the effective surface: project allowlist ∪ task
// extras, intersected with the spec-task-eligible catalogue so a stale or
// hand-crafted name can never widen it.
func eligibleSpecTaskTools(project *types.Project, task *types.SpecTask) []tool.Name {
	var out []tool.Name
	for _, name := range types.EffectiveAgentTools(project.AgentTools, task.AgentTools) {
		if mcptools.IsSpecTaskAgentTool(name) {
			out = append(out, name)
		}
	}
	return out
}
