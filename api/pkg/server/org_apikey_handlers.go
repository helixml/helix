package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// orgAPIKeyResponse extends ApiKey with the owner's email for display purposes.
type orgAPIKeyResponse struct {
	*types.ApiKey
	OwnerEmail string `json:"owner_email,omitempty"`
}

// listOrgAPIKeys godoc
// @Summary List organization API keys
// @Description List API keys for an organization. Owners see all keys, members see only their own.
// @Tags    organizations
// @Param   id path string true "Organization ID"
// @Success 200 {array} types.ApiKey
// @Router /api/v1/organizations/{id}/api_keys [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrgAPIKeys(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	ctx := r.Context()

	membership, err := apiServer.authorizeOrgMember(ctx, user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org member")
		http.Error(rw, "Not a member of this organization: "+err.Error(), http.StatusForbidden)
		return
	}

	query := &store.ListAPIKeysQuery{
		OrganizationID: orgID,
		Type:           types.APIkeytypeAPI,
	}

	// Non-owners can only see their own keys
	if membership.Role != types.OrganizationRoleOwner {
		query.Owner = user.ID
		query.OwnerType = user.Type
	}

	apiKeys, err := apiServer.Store.ListAPIKeys(ctx, query)
	if err != nil {
		log.Err(err).Msg("error listing org API keys")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter out spec-task-scoped keys and collect unique owner IDs
	ownerIDs := make(map[string]struct{})
	var filtered []*types.ApiKey
	for _, key := range apiKeys {
		if key.SpecTaskID != "" {
			continue
		}
		filtered = append(filtered, key)
		if key.Owner != "" {
			ownerIDs[key.Owner] = struct{}{}
		}
	}

	// Resolve owner IDs to emails
	emailByOwner := make(map[string]string, len(ownerIDs))
	for ownerID := range ownerIDs {
		u, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: ownerID})
		if err != nil {
			log.Warn().Err(err).Str("owner_id", ownerID).Msg("could not resolve API key owner")
			continue
		}
		emailByOwner[ownerID] = u.Email
	}

	result := make([]orgAPIKeyResponse, 0, len(filtered))
	for _, key := range filtered {
		result = append(result, orgAPIKeyResponse{
			ApiKey:     key,
			OwnerEmail: emailByOwner[key.Owner],
		})
	}

	writeResponse(rw, result, http.StatusOK)
}

// createOrgAPIKey godoc
// @Summary Create an organization API key
// @Description Create a new API key scoped to the organization. Any member can create keys.
// @Tags    organizations
// @Param   id path string true "Organization ID"
// @Param   request body object true "Request body with name"
// @Success 200 {object} types.ApiKey
// @Router /api/v1/organizations/{id}/api_keys [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createOrgAPIKey(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	ctx := r.Context()

	_, err := apiServer.authorizeOrgMember(ctx, user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org member")
		http.Error(rw, "Not a member of this organization: "+err.Error(), http.StatusForbidden)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(rw, "name is required", http.StatusBadRequest)
		return
	}

	keyStr, err := system.GenerateAPIKey()
	if err != nil {
		log.Err(err).Msg("error generating API key")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	apiKey := &types.ApiKey{
		Key:            keyStr,
		Name:           req.Name,
		Type:           types.APIkeytypeAPI,
		Owner:          user.ID,
		OwnerType:      user.Type,
		OrganizationID: orgID,
	}

	created, err := apiServer.Store.CreateAPIKey(ctx, apiKey)
	if err != nil {
		log.Err(err).Msg("error creating org API key")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, created, http.StatusOK)
}

// deleteOrgAPIKey godoc
// @Summary Delete an organization API key
// @Description Delete an API key. Owners can delete any org key, members only their own.
// @Tags    organizations
// @Param   id path string true "Organization ID"
// @Param   key path string true "API key to delete"
// @Success 200 {string} string
// @Router /api/v1/organizations/{id}/api_keys/{key} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteOrgAPIKey(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["id"]
	keyStr := mux.Vars(r)["key"]
	ctx := r.Context()

	membership, err := apiServer.authorizeOrgMember(ctx, user, orgID)
	if err != nil {
		log.Err(err).Msg("error authorizing org member")
		http.Error(rw, "Not a member of this organization: "+err.Error(), http.StatusForbidden)
		return
	}

	// Look up the key
	fetchedKey, err := apiServer.Store.GetAPIKey(ctx, &types.ApiKey{Key: keyStr})
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(rw, "API key not found", http.StatusNotFound)
			return
		}
		log.Err(err).Msg("error getting API key")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Verify the key belongs to this org
	if fetchedKey.OrganizationID != orgID {
		http.Error(rw, "API key not found in this organization", http.StatusNotFound)
		return
	}

	// Non-owners can only delete their own keys
	if membership.Role != types.OrganizationRoleOwner {
		if fetchedKey.Owner != user.ID {
			http.Error(rw, "You can only delete your own API keys", http.StatusForbidden)
			return
		}
	}

	if err := apiServer.Store.DeleteAPIKey(ctx, fetchedKey.Key); err != nil {
		log.Err(err).Msg("error deleting org API key")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, map[string]string{"status": "deleted"}, http.StatusOK)
}
