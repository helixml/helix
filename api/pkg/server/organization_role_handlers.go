package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// listOrganizationRoles godoc
// @Summary List roles in an organization
// @Description List all roles in an organization. Organization members can list roles.
// @Tags    organizations
// @Success 200 {array} types.Role
// @Router /api/v1/organizations/{id}/roles [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrganizationRoles(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]

	// Check if user has access to view roles
	_, err := apiServer.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org member")
		http.Error(rw, "Could not authorize org member: "+err.Error(), http.StatusForbidden)
		return
	}

	roles, err := apiServer.Store.ListRoles(r.Context(), orgID)
	if err != nil {
		log.Err(err).Msg("error listing roles")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, roles, http.StatusOK)
}
