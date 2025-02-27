package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"

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
		writeErrResponse(rw, errors.New("app is not associated with an organization"), http.StatusBadRequest)
		return
	}

	// Authorize user to view this application's access grants
	err = apiServer.authorizeUserToResource(r.Context(), user, app.OrganizationID, app.ID, types.ResourceApplication, types.ActionGet)
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

	writeResponse(rw, grants, http.StatusOK)
}

// createAppAccessGrant godoc
// @Summary Grant access to an app to a team or organization member
// @Description Grant access to an app to a team or organization member (organization owners can grant access to teams and organization members)
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
		writeErrResponse(rw, errors.New("app is not associated with an organization"), http.StatusBadRequest)
		return
	}

	var req types.CreateAccessGrantRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeErrResponse(rw, err, http.StatusBadRequest)
		return
	}

	// At least one must be set
	if req.UserID == "" && req.TeamID == "" {
		writeErrResponse(rw, errors.New("either user_id or team_id must be specified"), http.StatusBadRequest)
		return
	}

	// Both cannot be set as well
	if req.UserID != "" && req.TeamID != "" {
		writeErrResponse(rw, errors.New("either user_id or team_id must be specified, not both"), http.StatusBadRequest)
		return
	}

	// Check roles
	for _, role := range req.Roles {
		if role.ID == "" {
			writeErrResponse(rw, errors.New("role id is required"), http.StatusBadRequest)
			return
		}
	}

	// Authorize user to update application's memberships
	err = apiServer.authorizeUserToResource(r.Context(), user, app.OrganizationID, app.ID, types.ResourceAccessGrants, types.ActionUpdate)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	grants, err := apiServer.Store.CreateAccessGrant(r.Context(), &types.AccessGrant{
		OrganizationID: app.OrganizationID,
		ResourceID:     app.ID,
		UserID:         req.UserID,
		TeamID:         req.TeamID,
	}, req.Roles)
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	writeResponse(rw, grants, http.StatusOK)
}
