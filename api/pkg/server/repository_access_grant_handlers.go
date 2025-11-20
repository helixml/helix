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

// authorizeUserToRepositoryAccessGrants checks if the user can manage access grants for a repository
func (apiServer *HelixAPIServer) authorizeUserToRepositoryAccessGrants(ctx context.Context, user *types.User, repository *types.GitRepository) error {
	// Repository owner always has access
	if user.ID == repository.OwnerID {
		return nil
	}

	// Organization owners also have access
	if repository.OrganizationID != "" {
		_, err := apiServer.authorizeOrgOwner(ctx, user, repository.OrganizationID)
		if err == nil {
			return nil
		}
	}

	return fmt.Errorf("only repository owner or organization owner can manage access grants")
}

// listRepositoryAccessGrants godoc
// @Summary List repository access grants
// @Description List access grants for a git repository (repository owners can list access grants)
// @Tags    gitrepositories
// @Success 200 {array} types.AccessGrant
// @Router /api/v1/git/repositories/{id}/access-grants [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listRepositoryAccessGrants(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	repositoryID := mux.Vars(r)["id"]

	repository, err := apiServer.Store.GetGitRepository(r.Context(), repositoryID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	// Authorize user to view this repository's access grants
	err = apiServer.authorizeUserToRepositoryAccessGrants(r.Context(), user, repository)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	// Repositories without organization ID cannot have access grants
	if repository.OrganizationID == "" {
		writeErrResponse(rw, errors.New("repository is not associated with an organization"), http.StatusBadRequest)
		return
	}

	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: repository.OrganizationID,
		ResourceID:     repository.ID,
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

// createRepositoryAccessGrant godoc
// @Summary Grant access to a repository to a user
// @Description Grant access to a repository to a user (repository owners can grant access)
// @Tags    gitrepositories
// @Success 200 {object} types.AccessGrant
// @Param request body types.CreateAccessGrantRequest true "Request body with user reference and roles"
// @Router /api/v1/git/repositories/{id}/access-grants [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createRepositoryAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	repositoryID := mux.Vars(r)["id"]

	repository, err := apiServer.Store.GetGitRepository(r.Context(), repositoryID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErrResponse(rw, err, http.StatusNotFound)
			return
		}
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	var req types.CreateAccessGrantRequest
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeErrResponse(rw, err, http.StatusBadRequest)
		return
	}

	// Only user grants supported for repositories (no teams yet)
	if req.UserReference == "" {
		writeErrResponse(rw, errors.New("user_reference must be specified"), http.StatusBadRequest)
		return
	}

	// Authorize user to create access grants for this repository
	err = apiServer.authorizeUserToRepositoryAccessGrants(r.Context(), user, repository)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	// Repositories without organization ID cannot have access grants
	if repository.OrganizationID == "" {
		writeErrResponse(rw, errors.New("repository is not associated with an organization"), http.StatusBadRequest)
		return
	}

	roles, err := apiServer.ensureRoles(r.Context(), repository.OrganizationID, req.Roles)
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	var userID string

	// Find user by reference
	query := &store.GetUserQuery{}

	if strings.Contains(req.UserReference, "@") {
		query.Email = req.UserReference
	} else {
		query.ID = req.UserReference
	}

	targetUser, err := apiServer.Store.GetUser(r.Context(), query)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error getting user '%s': %w", req.UserReference, err), http.StatusInternalServerError)
		return
	}

	userID = targetUser.ID

	grant, err := apiServer.Store.CreateAccessGrant(r.Context(), &types.AccessGrant{
		OrganizationID: repository.OrganizationID,
		ResourceID:     repository.ID,
		UserID:         userID,
	}, roles)
	if err != nil {
		writeErrResponse(rw, fmt.Errorf("error creating access grant: %w", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, grant, http.StatusOK)
}

func (apiServer *HelixAPIServer) deleteRepositoryAccessGrant(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	repositoryID := mux.Vars(r)["id"]
	grantID := mux.Vars(r)["grant_id"]

	repository, err := apiServer.Store.GetGitRepository(r.Context(), repositoryID)
	if err != nil {
		writeErrResponse(rw, err, http.StatusInternalServerError)
		return
	}

	// Authorize user to delete access grants for this repository
	err = apiServer.authorizeUserToRepositoryAccessGrants(r.Context(), user, repository)
	if err != nil {
		writeErrResponse(rw, err, http.StatusForbidden)
		return
	}

	// Repositories without organization ID cannot have access grants
	if repository.OrganizationID == "" {
		writeErrResponse(rw, errors.New("repository is not associated with an organization"), http.StatusBadRequest)
		return
	}

	// Get access grant
	grants, err := apiServer.Store.ListAccessGrants(r.Context(), &store.ListAccessGrantsQuery{
		OrganizationID: repository.OrganizationID,
		ResourceID:     repository.ID,
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
