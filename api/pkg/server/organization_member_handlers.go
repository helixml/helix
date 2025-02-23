package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listOrganizationMembers godoc
// @Summary List organization members
// @Description List members of an organization
// @Tags    organizations
// @Success 200 {array} types.OrganizationMembership
// @Router /api/v1/organizations/{id}/members [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrganizationMembers(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]

	// Check if user has access to view members
	err := apiServer.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	members, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
		OrganizationID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error listing organization members")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, members, http.StatusOK)
}

// addOrganizationMember godoc
// @Summary Add an organization member
// @Description Add a member to an organization
// @Tags    organizations
// @Success 200 {object} types.OrganizationMembership
// @Router /api/v1/organizations/{id}/members [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) addOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]

	// Check if user has access to view members
	err := apiServer.authorizeOrgMember(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.AddOrganizationMemberRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Find user
	newMember, err := apiServer.Store.GetUser(r.Context(), &store.GetUserQuery{
		Email: req.UserEmail,
	})
	if err != nil {
		log.Err(err).Msg("error getting user")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create membership
	membership, err := apiServer.Store.CreateOrganizationMembership(r.Context(), &types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         newMember.ID,
		Role:           types.OrganizationRoleMember,
	})
	if err != nil {
		log.Err(err).Msg("error creating organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, membership, http.StatusCreated)
}
