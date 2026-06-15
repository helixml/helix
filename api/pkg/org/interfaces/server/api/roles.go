package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// ---- Roles --------------------------------------------------------------

// createRole creates a new Role row.
//
// @Summary Helix-org: create a role
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization slug or id"
// @Param payload body api.CreateRoleRequest true "Role spec"
// @Success 201 {object} api.RoleDTO
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/roles [post]
func (a *apiHandler) createRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreateRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("id is required"))
		return
	}
	// The service unions the caller's tools with the universal read
	// baseline — same merge the MCP create_role tool applies. Without
	// this, the chart UI's "New Role" dialog (no tools picker) would
	// create Roles with empty tool lists and every Worker holding them
	// would have no MCP surface at all.
	rl, err := a.deps.Roles.Create(r.Context(), orgID, roles.CreateParams{
		ID:      strings.TrimSpace(req.ID),
		Content: req.Content,
		Tools:   toToolNames(req.Tools),
		Streams: toStreamIDs(req.Streams),
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("create role: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, roleDTO(rl))
}

// getRole returns one Role.
//
// @Summary Helix-org: get a role
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization slug or id"
// @Param id path string true "Role ID"
// @Success 200 {object} api.RoleDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/roles/{id} [get]
func (a *apiHandler) getRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.RoleID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("role id is required"))
		return
	}
	rl, err := a.deps.Queries.GetRole(r.Context(), orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get role %s: %w", id, err))
		return
	}
	writeJSON(w, http.StatusOK, roleDTO(rl))
}

// updateRole rewrites a Role's content / tools / streams.
//
// @Summary Helix-org: update a role
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization slug or id"
// @Param id path string true "Role ID"
// @Param payload body api.UpdateRoleRequest true "Patch fields"
// @Success 200 {object} api.RoleDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/roles/{id} [put]
func (a *apiHandler) updateRole(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.RoleID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("role id is required"))
		return
	}
	var req UpdateRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var toolsPatch *[]tool.Name
	if req.Tools != nil {
		t := toToolNames(req.Tools)
		toolsPatch = &t
	}
	var streamsPatch *[]streaming.StreamID
	if req.Streams != nil {
		s := toStreamIDs(req.Streams)
		streamsPatch = &s
	}
	updated, err := a.deps.Roles.Update(r.Context(), orgID, id, roles.UpdateParams{
		Content: req.Content,
		Tools:   toolsPatch,
		Streams: streamsPatch,
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update role: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, roleDTO(updated))
}

// deleteRole fires every Worker holding the Role then removes the
// Role row. Refuses to act on r-owner (409).
//
// @Summary Helix-org: delete a role (cascade-fires its workers)
// @Tags HelixOrg
// @Param org path string true "Organization slug or id"
// @Param id path string true "Role ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/roles/{id} [delete]
func (a *apiHandler) deleteRole(w http.ResponseWriter, r *http.Request) {
	if a.deps.Lifecycle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("lifecycle is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.RoleID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("role id is required"))
		return
	}
	switch err := a.deps.Lifecycle.DeleteRole(r.Context(), orgID, id); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, errStatus(err), err)
	}
}

// ---- helpers ------------------------------------------------------------

func toToolNames(in []string) []tool.Name {
	if len(in) == 0 {
		return nil
	}
	out := make([]tool.Name, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, tool.Name(t))
		}
	}
	return out
}

func toStreamIDs(in []string) []streaming.StreamID {
	if len(in) == 0 {
		return nil
	}
	out := make([]streaming.StreamID, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, streaming.StreamID(t))
		}
	}
	return out
}

// listTools returns the catalogue of available MCP tools that can be
// listed on a Role. Powers the role editor's multi-select.
//
// @Summary Helix-org: list available MCP tools
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.ToolDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/tools [get]
func (a *apiHandler) listTools(w http.ResponseWriter, r *http.Request) {
	out := make([]ToolDTO, 0)
	if a.deps.Tools != nil {
		for _, t := range a.deps.Tools.List() {
			out = append(out, ToolDTO{
				Name:        string(t.Name()),
				Description: t.Description(),
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// listRoles returns every Role row.
//
// @Summary Helix-org: list roles
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.RoleDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/roles [get]
func (a *apiHandler) listRoles(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	roles, err := a.deps.Queries.ListRoles(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list roles: %w", err))
		return
	}
	out := make([]RoleDTO, 0, len(roles))
	for _, ro := range roles {
		out = append(out, roleDTO(ro))
	}
	writeJSON(w, http.StatusOK, out)
}

func roleDTO(r orgchart.Role) RoleDTO {
	dto := RoleDTO{ID: string(r.ID), Content: r.Content}
	if !r.CreatedAt.IsZero() {
		dto.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if !r.UpdatedAt.IsZero() {
		dto.UpdatedAt = r.UpdatedAt.Format(time.RFC3339)
	}
	for _, t := range r.Tools {
		dto.Tools = append(dto.Tools, string(t))
	}
	for _, s := range r.Streams {
		dto.Streams = append(dto.Streams, string(s))
	}
	return dto
}
