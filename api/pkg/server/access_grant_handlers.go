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

// listAppAccessGrants godoc
// @Summary List app access grants
// @Description List access grants for an app (organization owners and members can list access grants)
// @Tags    apps
// @Success 200 {array} types.AccessGrant
// @Router /api/v1/apps/{id}/access-grants [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listAppAccessGrants(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	appID := mux.Vars(r)["id"]

	app, err := apiServer.Store.GetApp(r.Context(), appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	if app.OrganizationID == "" {
		writeErrResponse(rw, errors.New("agent is not associated with an organization"), http.StatusBadRequest)
		return
	}

	// Authorize user to view this application's access grants
	err = apiServer.authorizeUserToAppAccessGrants(r.Context(), user, app, types.ActionGet)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
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

// createAppAccessGrant godoc
// @Summary Grant access to an agent to a team or organization member
// @Description Grant access to an agent to a team or organization member (organization owners can grant access to teams and organization members)
// @Tags    apps
// @Success 200 {object} types.AccessGrant
// @Param request body types.CreateAccessGrantRequest true "Request body with team or organization member ID and role"
// @Router /api/v1/apps/{id}/access-grants [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createAppAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	appID := mux.Vars(r)["id"]

	app, err := apiServer.Store.GetApp(r.Context(), appID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	if app.OrganizationID == "" {
		writeErrResponse(rw, errors.New("agent is not associated with an organization"), http.StatusBadRequest)
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

	// Authorize user to update application's memberships
	err = apiServer.authorizeUserToAppAccessGrants(r.Context(), user, app, types.ActionUpdate)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	roles, err := apiServer.ensureRoles(r.Context(), app.OrganizationID, req.Roles)
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
		newMember, err := apiServer.Store.GetUser(r.Context(), query)
		if err != nil {
			writeErrResponse(rw, fmt.Errorf("error getting user '%s': %w", req.UserReference, err), http.StatusInternalServerError)
			return
		}

		userID = newMember.ID
	}

	// If team ID is set, check if it exists in the organization
	if req.TeamID != "" {
		_, err := apiServer.Store.GetTeam(r.Context(), &store.GetTeamQuery{
			OrganizationID: app.OrganizationID,
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

	grants, err := apiServer.Store.CreateAccessGrant(r.Context(), &types.AccessGrant{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
		UserID:         userID,
		TeamID:         req.TeamID,
	}, roles)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error creating access grant: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, grants, http.StatusOK)
}

// ensureRoles converts role names into role objects for access grants
func (apiServer *HelixAPIServer) ensureRoles(ctx context.Context, orgID string, roles []string) ([]*types.Role, error) {
	orgRoles, err := apiServer.Store.ListRoles(ctx, orgID)
	if err != nil {
		return nil, err
	}

	orgRolesMap := make(map[string]*types.Role)
	for _, role := range orgRoles {
		orgRolesMap[role.Name] = role
	}

	var resp []*types.Role

	for _, role := range roles {
		role, ok := orgRolesMap[role]
		if !ok {
			return nil, fmt.Errorf("role '%s' not found", role)
		}

		resp = append(resp, role)
	}

	return resp, nil
}

func (apiServer *HelixAPIServer) deleteAppAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	appID := mux.Vars(r)["id"]
	grantID := mux.Vars(r)["grant_id"]

	app, err := apiServer.Store.GetApp(r.Context(), appID)
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	// Authorize user to update application's memberships
	err = apiServer.authorizeUserToAppAccessGrants(r.Context(), user, app, types.ActionUpdate)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	// Get access grant
	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
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
