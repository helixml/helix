package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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

// updateRole rewrites a Role's content / tools / streams. Each field
// is optional in the body — omit to leave untouched. Tools/Streams are
// REPLACED wholesale when provided (pass `[]` to clear).
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

// ---- Positions ----------------------------------------------------------

// createPosition creates a new Position row.
//
// @Summary Helix-org: create a position
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization slug or id"
// @Param payload body api.CreatePositionRequest true "Position spec"
// @Success 201 {object} api.PositionDTO
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/positions [post]
func (a *apiHandler) createPosition(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreatePositionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.ID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("id is required"))
		return
	}
	if strings.TrimSpace(req.RoleID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("role_id is required"))
		return
	}
	var parent *orgchart.PositionID
	if p := strings.TrimSpace(req.ParentID); p != "" {
		pid := orgchart.PositionID(p)
		parent = &pid
	}
	pos, err := orgchart.NewPosition(orgchart.PositionID(strings.TrimSpace(req.ID)), orgchart.RoleID(strings.TrimSpace(req.RoleID)), parent, orgID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Store.Positions.Create(r.Context(), pos); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("create position: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, positionDTO(pos))
}

// getPosition returns one Position.
//
// @Summary Helix-org: get a position
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization slug or id"
// @Param id path string true "Position ID"
// @Success 200 {object} api.PositionDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/positions/{id} [get]
func (a *apiHandler) getPosition(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.PositionID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("position id is required"))
		return
	}
	pos, err := a.deps.Store.Positions.Get(r.Context(), orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get position %s: %w", id, err))
		return
	}
	writeJSON(w, http.StatusOK, positionDTO(pos))
}

// updatePosition re-points the position's parent and/or role.
//
// @Summary Helix-org: update a position
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization slug or id"
// @Param id path string true "Position ID"
// @Param payload body api.UpdatePositionRequest true "Patch fields"
// @Success 200 {object} api.PositionDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/positions/{id} [put]
func (a *apiHandler) updatePosition(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.PositionID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("position id is required"))
		return
	}
	var req UpdatePositionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, err := a.deps.Store.Positions.Get(r.Context(), orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get position %s: %w", id, err))
		return
	}
	if req.RoleID != nil {
		existing.RoleID = orgchart.RoleID(strings.TrimSpace(*req.RoleID))
	}
	if req.ParentID != nil {
		if p := strings.TrimSpace(*req.ParentID); p == "" {
			existing.ParentID = nil
		} else {
			pid := orgchart.PositionID(p)
			existing.ParentID = &pid
		}
	}
	if err := a.deps.Store.Positions.Update(r.Context(), existing); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update position: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, positionDTO(existing))
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
