package server

import (
	"context"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/types"
)

// listAgentToolCatalogue godoc
// @Summary List the Helix MCP tools that can be granted to spec tasks
// @Description Returns the catalogue backing the project and task tool pickers. The set is static per deployment; a project grants a subset to all its tasks and a task may add more on top.
// @Tags spec-driven-tasks
// @Produce json
// @Success 200 {array} types.AgentToolInfo
// @Router /api/v1/agent-tools [get]
// @Security BearerAuth
func (s *HelixAPIServer) listAgentToolCatalogue(w http.ResponseWriter, r *http.Request) {
	out := make([]types.AgentToolInfo, 0, len(mcptools.SpecTaskAgentTools))
	if s.helixOrg != nil && s.helixOrg.mcpServer != nil {
		for _, t := range s.helixOrg.mcpServer.ToolCatalogue(mcptools.SpecTaskAgentTools) {
			out = append(out, types.AgentToolInfo{Name: t.Name, Description: t.Description})
		}
	}
	writeResponse(w, out, http.StatusOK)
}

// sanitizeAgentTools drops anything outside the spec-task-eligible catalogue
// before persisting. The MCP backend intersects again at call time, so this is
// about keeping the stored row honest — what the picker shows is what runs.
func sanitizeAgentTools(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !mcptools.IsSpecTaskAgentTool(name) {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// specTaskAgentTools is the task's org tool surface as strings; nil for
// non-org spec-task sessions keeps the context server out of those sandboxes.
func (s *HelixAPIServer) specTaskAgentTools(ctx context.Context, session *types.Session) []string {
	if session == nil || session.Metadata.SpecTaskID == "" {
		return nil
	}
	task, err := s.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
	if err != nil {
		log.Warn().Err(err).Str("spec_task_id", session.Metadata.SpecTaskID).Msg("agent tools: failed to load spec task")
		return nil
	}
	project, err := s.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		log.Warn().Err(err).Str("project_id", task.ProjectID).Msg("agent tools: failed to load project")
		return nil
	}
	if project.OrganizationID == "" {
		return nil
	}
	own, _ := s.specTaskToolSurface(ctx, project, task)
	out := make([]string, 0, len(own))
	for _, name := range own {
		out = append(out, string(name))
	}
	return out
}
