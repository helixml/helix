package server

import (
	"encoding/json"
	"net/http"
	"strings"

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
	_, err := apiServer.authorizeOrgMember(r.Context(), user, orgID)
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
// @Param request    body types.AddOrganizationMemberRequest true "Request body with user email to add.")
// @Router /api/v1/organizations/{id}/members [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) addOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]

	// Check if user has owner permissions (not just membership)
	_, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Only organization owners can add members: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.AddOrganizationMemberRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := &store.GetUserQuery{}

	if strings.Contains(req.UserReference, "@") {
		query.Email = req.UserReference
	} else {
		query.ID = req.UserReference
	}

	// Find user
	newMember, err := apiServer.Store.GetUser(r.Context(), query)
	if err != nil {
		log.Err(err).Msg("error getting user")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Role == "" {
		req.Role = types.OrganizationRoleMember
	}

	// Create membership
	membership, err := apiServer.Store.CreateOrganizationMembership(r.Context(), &types.OrganizationMembership{
		OrganizationID: orgID,
		UserID:         newMember.ID,
		Role:           req.Role,
	})
	if err != nil {
		log.Err(err).Msg("error creating organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, membership, http.StatusCreated)
}

// removeOrganizationMember godoc
// @Summary Remove an organization member
// @Description Remove a member from an organization
// @Tags    organizations
// @Success 200
// @Router /api/v1/organizations/{id}/members/{user_id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) removeOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	userIDToRemove := mux.Vars(r)["user_id"]

	// Check if user has owner permissions (not just membership)
	_, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Only organization owners can remove members: "+err.Error(), http.StatusForbidden)
		return
	}

	// Get the membership we're trying to remove
	memberToRemove, err := apiServer.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userIDToRemove,
	})
	if err != nil {
		log.Err(err).Msg("error getting organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If the member is an owner, check if they're the last owner
	if memberToRemove.Role == types.OrganizationRoleOwner {
		// Get all owners in the organization
		allMembers, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
			OrganizationID: orgID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization members")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Count owners
		ownerCount := 0
		for _, member := range allMembers {
			if member.Role == types.OrganizationRoleOwner {
				ownerCount++
			}
		}

		// If this is the last owner, prevent deletion
		if ownerCount <= 1 {
			log.Warn().Msg("attempted to remove the last owner of an organization")
			http.Error(rw, "Cannot remove the last owner of an organization", http.StatusBadRequest)
			return
		}
	}

	// Delete membership (this will cascade delete team memberships in the store layer)
	err = apiServer.Store.DeleteOrganizationMembership(r.Context(), orgID, userIDToRemove)
	if err != nil {
		log.Err(err).Msg("error removing organization member")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, nil, http.StatusOK)
}

// updateOrganizationMember godoc
// @Summary Update an organization member
// @Description Update a member's role in an organization
// @Tags    organizations
// @Success 200 {object} types.OrganizationMembership
// @Param request    body types.UpdateOrganizationMemberRequest true "Request body with role to update to.")
// @Router /api/v1/organizations/{id}/members/{user_id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateOrganizationMember(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	userIDToUpdate := mux.Vars(r)["user_id"]

	// Check if user has access to modify members (needs to be an owner)
	_, err := apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusForbidden)
		return
	}

	var req types.UpdateOrganizationMemberRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing membership
	membership, err := apiServer.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         userIDToUpdate,
	})
	if err != nil {
		log.Err(err).Msg("error getting organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If changing from owner to member, check if they're the last owner
	if membership.Role == types.OrganizationRoleOwner && req.Role != types.OrganizationRoleOwner {
		// Get all owners in the organization
		allMembers, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
			OrganizationID: orgID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization members")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Count owners
		ownerCount := 0
		for _, member := range allMembers {
			if member.Role == types.OrganizationRoleOwner {
				ownerCount++
			}
		}

		// If this is the last owner, prevent role change
		if ownerCount <= 1 {
			log.Warn().Msg("attempted to change the role of the last owner of an organization")
			http.Error(rw, "Cannot change the role of the last owner of an organization", http.StatusBadRequest)
			return
		}
	}

	// Update role
	membership.Role = req.Role

	// Save updated membership
	updatedMembership, err := apiServer.Store.UpdateOrganizationMembership(r.Context(), membership)
	if err != nil {
		log.Err(err).Msg("error updating organization membership")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, updatedMembership, http.StatusOK)
}
