package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
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
	if a.deps.Now == nil {
		writeError(w, http.StatusInternalServerError, errors.New("api not configured (missing Now)"))
		return
	}
	tools := toToolNames(req.Tools)
	streams := toStreamIDs(req.Streams)
	rl, err := orgchart.NewRole(orgchart.RoleID(strings.TrimSpace(req.ID)), req.Content, tools, streams, a.deps.Now().UTC(), orgID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Store.Roles.Create(r.Context(), rl); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("create role: %w", err))
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
	rl, err := a.deps.Store.Roles.Get(r.Context(), orgID, id)
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
	existing, err := a.deps.Store.Roles.Get(r.Context(), orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get role %s: %w", id, err))
		return
	}
	if req.Content != nil {
		existing.Content = *req.Content
	}
	if req.Tools != nil {
		existing.Tools = toToolNames(req.Tools)
	}
	if req.Streams != nil {
		existing.Streams = toStreamIDs(req.Streams)
	}
	if a.deps.Now != nil {
		existing.UpdatedAt = a.deps.Now().UTC()
	} else {
		existing.UpdatedAt = time.Now().UTC()
	}
	if err := a.deps.Store.Roles.Update(r.Context(), existing); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("update role: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, roleDTO(existing))
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
	case errors.Is(err, lifecycle.ErrOwnerRoleProtected), errors.Is(err, lifecycle.ErrOwnerProtected):
		writeError(w, http.StatusConflict, err)
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
