package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// authorizeUserToProjectAccessGrants checks if the user can manage access grants for a project
func (apiServer *HelixAPIServer) authorizeUserToProjectAccessGrants(ctx context.Context, user *types.User, project *types.Project, action types.Action) error {
	// Check if user is a member of the org
	orgMembership, err := apiServer.authorizeOrgMember(ctx, user, project.OrganizationID)
	if err != nil {
		return err
	}

	// Project owner can always manage access grants
	if user.ID == project.UserID {
		return nil
	}

	// Org owner can always manage access grants
	if orgMembership.Role == types.OrganizationRoleOwner {
		return nil
	}

	return apiServer.authorizeUserToResource(ctx, user, project.OrganizationID, project.ID, types.ResourceAccessGrants, action)
}

// listProjectAccessGrants godoc
// @Summary List project access grants
// @Description List access grants for a project (project owners and org owners can list access grants)
// @Tags    projects
// @Success 200 {array} types.AccessGrant
// @Router /api/v1/projects/{id}/access-grants [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listProjectAccessGrants(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	projectID := mux.Vars(r)["id"]

	project, err := apiServer.Store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	if project.OrganizationID == "" {
		writeErrResponse(rw, errors.New("project is not associated with an organization"), http.StatusBadRequest)
		return
	}

	// Authorize user to view this project's access grants
	err = apiServer.authorizeUserToProjectAccessGrants(r.Context(), user, project, types.ActionGet)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: project.OrganizationID,
		ResourceID:     project.ID,
	})
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	for _, grant := range grants {
		if grant.UserID != "" {
			grantUser, err := apiServer.Store.GetUser(r.Context(), &store.GetUserQuery{ID: grant.UserID})
			if err != nil {
				log.Error().Err(err).Str("user_id", grant.UserID).Msg("error getting user for access grant")
				continue
			}

			grant.User = *grantUser
		}
	}

	writeResponse(rw, grants, http.StatusOK)
}

// createProjectAccessGrant godoc
// @Summary Grant access to a project to a team or organization member
// @Description Grant access to a project to a team or organization member (project owners and org owners can grant access)
// @Tags    projects
// @Success 200 {object} types.AccessGrant
// @Param request body types.CreateAccessGrantRequest true "Request body with team or organization member ID and roles"
// @Router /api/v1/projects/{id}/access-grants [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createProjectAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	projectID := mux.Vars(r)["id"]

	project, err := apiServer.Store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	if project.OrganizationID == "" {
		writeErrResponse(rw, errors.New("project is not associated with an organization"), http.StatusBadRequest)
		return
	}

	var req types.CreateAccessGrantRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeErrResponse(rw, err, http.StatusBadRequest)
		return
	}

	// At least one must be set
	if req.UserReference == "" && req.TeamID == "" {
		writeErrResponse(rw, errors.New("either user_reference or team_id must be specified"), http.StatusBadRequest)
		return
	}

	// Both cannot be set as well
	if req.UserReference != "" && req.TeamID != "" {
		writeErrResponse(rw, errors.New("either user_reference or team_id must be specified, not both"), http.StatusBadRequest)
		return
	}

	// Authorize user to create access grants for this project
	err = apiServer.authorizeUserToProjectAccessGrants(r.Context(), user, project, types.ActionCreate)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	roles, err := apiServer.ensureRoles(r.Context(), project.OrganizationID, req.Roles)
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	var userID string

	// If user reference is set, find the user based on either email or user ID
	if req.UserReference != "" {
		query := &store.GetUserQuery{}

		if strings.Contains(req.UserReference, "@") {
			query.Email = req.UserReference
		} else {
			query.ID = req.UserReference
		}

		// Find user
		targetUser, err := apiServer.Store.GetUser(r.Context(), query)
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("error getting user '%s': %w", req.UserReference, err), http.StatusInternalServerError)
			return
		}

		userID = targetUser.ID
	}

	// If team ID is set, check if it exists in the organization
	if req.TeamID != "" {
		_, err := apiServer.Store.GetTeam(r.Context(), &store.GetTeamQuery{
			OrganizationID: project.OrganizationID,
			ID:             req.TeamID,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErrResponse(rw, fmt.Errorf("team '%s' not found", req.TeamID), http.StatusBadRequest)
				return
			}

			writeErrResponse(rw, fmt.Errorf("error getting team '%s': %w", req.TeamID, err), http.StatusInternalServerError)
			return
		}
	}

	grant, err := apiServer.Store.CreateAccessGrant(r.Context(), &types.AccessGrant{
		OrganizationID: project.OrganizationID,
		ResourceID:     project.ID,
		UserID:         userID,
		TeamID:         req.TeamID,
	}, roles)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error creating access grant: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, grant, http.StatusOK)
}

func (apiServer *HelixAPIServer) deleteProjectAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	projectID := mux.Vars(r)["id"]
	grantID := mux.Vars(r)["grant_id"]

	project, err := apiServer.Store.GetProject(r.Context(), projectID)
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	// Authorize user to delete access grants for this project
	err = apiServer.authorizeUserToProjectAccessGrants(r.Context(), user, project, types.ActionDelete)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	// Get access grant
	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: project.OrganizationID,
		ResourceID:     project.ID,
	})
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	for _, grant := range grants {
		if grant.ID == grantID {
			err = apiServer.Store.DeleteAccessGrant(r.Context(), grantID)
			if err != nil {
				writeErrResponse(rw, err, http.StatusInternalServerError)
				return
			}

			// All good, return
			return
		}
	}

	writeErrResponse(rw, errors.New("access grant not found"), http.StatusNotFound)
}
