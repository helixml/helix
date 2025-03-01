package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listOrganizations godoc
// @Summary List organizations
// @Description List organizations
// @Tags    providers

// @Success 200 {array} types.Organization
// @Router /api/v1/organizations [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrganizations(rw http.ResponseWriter, r *http.Request) {
	organizations, err := apiServer.Store.ListOrganizations(r.Context(), &store.ListOrganizationsQuery{})
	if err != nil {
		log.Err(err).Msg("error listing organizations")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, organizations, http.StatusOK)
}

// getOrganization godoc
// @Summary Get an organization
// @Description Get an organization
// @Tags    organizations

// @Success 200 {object} types.Organization
// @Router /api/v1/organizations/{id} [get]
func (apiServer *HelixAPIServer) getOrganization(rw http.ResponseWriter, r *http.Request) {
	reference := mux.Vars(r)["id"]

	q := &store.GetOrganizationQuery{}

	// If reference starts with org prefix, then query by ID, otherwise query by name
	if strings.HasPrefix(reference, system.OrganizationPrefix) {
		q.ID = reference
	} else {
		q.Name = reference
	}

	organization, err := apiServer.Store.GetOrganization(r.Context(), q)
	if err != nil {
		log.Err(err).Msg("error getting organization")
		http.Error(rw, "Could not get organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, organization, http.StatusOK)
}

// createOrganization godoc
// @Summary Create a new organization
// @Description Create a new organization
// @Tags    organizations
// @Param request    body types.Organization true "Request body with organization configuration.")
// @Success 200 {object} types.Organization
// @Router /api/v1/organizations [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createOrganization(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	organization := &types.Organization{}
	err := json.NewDecoder(r.Body).Decode(organization)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	if organization.Name == "" {
		http.Error(rw, "Name not specified", http.StatusBadRequest)
		return
	}

	organization.Owner = user.ID

	ctx := context.Background()

	createdOrg, err := apiServer.Store.CreateOrganization(ctx, organization)
	if err != nil {
		log.Err(err).Msg("error creating organization")
		http.Error(rw, "Could not create organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create an org membership for the user (owner role)
	_, err = apiServer.Store.CreateOrganizationMembership(ctx, &types.OrganizationMembership{
		OrganizationID: createdOrg.ID,
		UserID:         user.ID,
		Role:           types.OrganizationRoleOwner,
	})
	if err != nil {
		log.Err(err).Msg("error creating organization membership")
		http.Error(rw, "Could not create organization membership: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Seed the roles
	err = apiServer.seedOrganizationRoles(ctx, createdOrg)
	if err != nil {
		log.Err(err).Msg("error seeding organization roles")
		http.Error(rw, "Could not seed organization roles: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, createdOrg, http.StatusCreated)
}

func (apiServer *HelixAPIServer) seedOrganizationRoles(ctx context.Context, org *types.Organization) error {
	for _, role := range types.Roles {
		orgRole := &types.Role{
			ID:             system.GenerateRoleID(),
			OrganizationID: org.ID,
			Name:           role.Name,
			Description:    role.Description,
			Config:         role.Config,
		}

		_, err := apiServer.Store.CreateRole(ctx, orgRole)
		if err != nil {
			return fmt.Errorf("error creating organization role: %w", err)
		}
	}

	return nil
}

// deleteOrganization godoc
// @Summary Delete an organization
// @Description Delete an organization, must be an owner of the organization
// @Tags    organizations

// @Success 200 {object} types.Organization
// @Router /api/v1/organizations/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteOrganization(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	orgID := mux.Vars(r)["id"]

	// Check if org exists
	_, err := apiServer.Store.GetOrganization(r.Context(), &store.GetOrganizationQuery{
		ID: orgID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "Organization not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error getting organization")
		http.Error(rw, "Could not get organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user is owner
	_, err = apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = apiServer.Store.DeleteOrganization(r.Context(), orgID)
	if err != nil {
		log.Err(err).Msg("error deleting organization")
		http.Error(rw, "Could not delete organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rw.WriteHeader(http.StatusOK)
}

// updateOrganization godoc
// @Summary Update an organization
// @Description Update an organization, must be an owner of the organization
// @Tags    organizations
// @Param request    body types.Organization true "Request body with organization configuration.")
// @Success 200 {object} types.Organization
// @Router /api/v1/organizations/{id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateOrganization(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	orgID := mux.Vars(r)["id"]

	updatedOrganization := &types.Organization{}
	err := json.NewDecoder(r.Body).Decode(updatedOrganization)
	if err != nil {
		log.Err(err).Msg("error decoding request body")
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	_, err = apiServer.authorizeOrgOwner(r.Context(), user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org owner")
		http.Error(rw, "Could not authorize org owner: "+err.Error(), http.StatusInternalServerError)
		return
	}

	updatedOrganization.ID = orgID

	updatedOrganization, err = apiServer.Store.UpdateOrganization(r.Context(), updatedOrganization)
	if err != nil {
		log.Err(err).Msg("error updating organization")
		http.Error(rw, "Could not update organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, updatedOrganization, http.StatusOK)
}
