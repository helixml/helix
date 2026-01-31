package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	user := getRequestUser(r)

	// If user is not an admin, filter to only show organizations they're a member of
	if !isAdmin(user) {
		// Get memberships for the current user
		memberships, err := apiServer.Store.ListOrganizationMemberships(r.Context(), &store.ListOrganizationMembershipsQuery{
			UserID: user.ID,
		})
		if err != nil {
			log.Err(err).Msg("error listing organization memberships")
			http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// If user has no memberships, return empty array
		if len(memberships) == 0 {
			writeResponse(rw, []*types.Organization{}, http.StatusOK)
			return
		}

		// Get all organizations the user is a member of
		var organizations []*types.Organization
		for _, membership := range memberships {
			org, err := apiServer.Store.GetOrganization(r.Context(), &store.GetOrganizationQuery{
				ID: membership.OrganizationID,
			})
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					// Skip if org not found
					continue
				}
				log.Err(err).Msg("error getting organization")
				http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
				return
			}
			organizations = append(organizations, org)
		}

		writeResponse(rw, organizations, http.StatusOK)
		return
	}

	// For admin users, get all organizations (existing behavior)
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
// @Description Create a new organization. Only admin users can create organizations.
// @Tags    organizations
// @Param request    body types.Organization true "Request body with organization configuration.")
// @Success 200 {object} types.Organization
// @Router /api/v1/organizations [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createOrganization(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	// Check if user is admin if creation is turned off for non-admins
	if !apiServer.Cfg.Organizations.CreateEnabledForNonAdmins && !user.Admin {
		http.Error(rw, "Organization creation is disabled for non-admin users", http.StatusForbidden)
		return
	}

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

// @Success 200
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

	existingOrg, err := apiServer.Store.GetOrganization(r.Context(), &store.GetOrganizationQuery{
		ID: orgID,
	})
	if err != nil {
		log.Err(err).Msg("error getting organization")
		http.Error(rw, "Could not get organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	existingOrg.DisplayName = updatedOrganization.DisplayName
	existingOrg.Name = updatedOrganization.Name

	// Track guidelines changes with versioning
	if updatedOrganization.Guidelines != existingOrg.Guidelines {
		// Save current version to history before updating
		if existingOrg.Guidelines != "" {
			history := &types.GuidelinesHistory{
				ID:             system.GenerateUUID(),
				OrganizationID: orgID,
				Version:        existingOrg.GuidelinesVersion,
				Guidelines:     existingOrg.Guidelines,
				UpdatedBy:      existingOrg.GuidelinesUpdatedBy,
				UpdatedAt:      existingOrg.GuidelinesUpdatedAt,
			}
			if err := apiServer.Store.CreateGuidelinesHistory(r.Context(), history); err != nil {
				log.Warn().Err(err).Str("org_id", orgID).Msg("failed to save guidelines history")
			}
		}

		// Update guidelines with new version
		existingOrg.Guidelines = updatedOrganization.Guidelines
		existingOrg.GuidelinesVersion++
		existingOrg.GuidelinesUpdatedAt = time.Now()
		existingOrg.GuidelinesUpdatedBy = user.ID
	}

	existingOrg, err = apiServer.Store.UpdateOrganization(r.Context(), existingOrg)
	if err != nil {
		log.Err(err).Msg("error updating organization")
		http.Error(rw, "Could not update organization: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, existingOrg, http.StatusOK)
}

// getOrganizationGuidelinesHistory returns the history of guidelines changes for an organization
// @Summary Get organization guidelines history
// @Description Get the version history of guidelines for an organization
// @Tags Organizations
// @Accept json
// @Produce json
// @Param id path string true "Organization ID"
// @Success 200 {array} types.GuidelinesHistory
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/organizations/{id}/guidelines-history [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getOrganizationGuidelinesHistory(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	vars := mux.Vars(r)
	orgID := vars["id"]

	if orgID == "" {
		http.Error(rw, "Organization ID is required", http.StatusBadRequest)
		return
	}

	org, err := apiServer.Store.GetOrganization(r.Context(), &store.GetOrganizationQuery{ID: orgID})
	if err != nil {
		http.Error(rw, "Organization not found", http.StatusNotFound)
		return
	}

	// Check if user is a member of the organization
	_, err = apiServer.Store.GetOrganizationMembership(r.Context(), &store.GetOrganizationMembershipQuery{
		OrganizationID: org.ID,
		UserID:         user.ID,
	})
	if err != nil && !user.Admin {
		http.Error(rw, "Not authorized to view organization guidelines history", http.StatusForbidden)
		return
	}

	history, err := apiServer.Store.ListGuidelinesHistory(r.Context(), orgID, "", "")
	if err != nil {
		log.Error().
			Err(err).
			Str("org_id", orgID).
			Msg("failed to get organization guidelines history")
		http.Error(rw, "Failed to get guidelines history", http.StatusInternalServerError)
		return
	}

	// Populate user display names and emails
	for _, entry := range history {
		if entry.UpdatedBy != "" {
			if u, err := apiServer.Store.GetUser(r.Context(), &store.GetUserQuery{ID: entry.UpdatedBy}); err == nil && u != nil {
				entry.UpdatedByName = u.FullName
				entry.UpdatedByEmail = u.Email
			}
		}
	}

	writeResponse(rw, history, http.StatusOK)
}
