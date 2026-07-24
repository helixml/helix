package server

import (
	"errors"
	"net/http"

	jsoniter "github.com/json-iterator/go"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// ProvisionUserAPIKeyRequest is the body for the admin mint-api-key endpoint.
type ProvisionUserAPIKeyRequest struct {
	Email string `json:"email"`
}

// ProvisionUserAPIKeyResponse returns the resolved user and their API key.
type ProvisionUserAPIKeyResponse struct {
	UserID string `json:"user_id"`
	APIKey string `json:"api_key"`
}

// adminProvisionUserAPIKey resolves a user by email and reuses-or-mints an API
// key owned by that user, returning the user id and key. Admin-only (registered
// on adminRouter). This lets a trusted service (e.g. HelixOS) obtain a per-user
// key on behalf of a teammate without that person creating one — the same
// reuse-or-mint behaviour as helixorg.HelixAPIKeys.User, inlined to avoid the
// config-registry dependency.
//
// The user must already exist (they log in via OIDC first); we do not create
// users here. Idempotent: an existing "api" key is reused before minting.
func (apiServer *HelixAPIServer) adminProvisionUserAPIKey(_ http.ResponseWriter, req *http.Request) (*ProvisionUserAPIKeyResponse, error) {
	ctx := req.Context()

	user := getRequestUser(req)
	if !isAdmin(user) {
		return nil, system.NewHTTPError403("only admins can provision user API keys")
	}

	var request ProvisionUserAPIKeyRequest
	if err := jsoniter.NewDecoder(req.Body).Decode(&request); err != nil {
		return nil, system.NewHTTPError400("failed to decode request: " + err.Error())
	}
	if request.Email == "" {
		return nil, system.NewHTTPError400("email is required")
	}

	target, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{Email: request.Email})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("no Helix user with email " + request.Email + " — they must sign in once first")
		}
		return nil, system.NewHTTPError500("failed to look up user: " + err.Error())
	}

	// Reuse an existing api key before minting a new one.
	keys, err := apiServer.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner: target.ID,
		Type:  types.APIkeytypeAPI,
	})
	if err != nil {
		return nil, system.NewHTTPError500("failed to list api keys: " + err.Error())
	}
	if len(keys) > 0 {
		return &ProvisionUserAPIKeyResponse{UserID: target.ID, APIKey: keys[0].Key}, nil
	}

	keyStr, err := system.GenerateAPIKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to generate api key: " + err.Error())
	}
	if _, err := apiServer.Store.CreateAPIKey(ctx, &types.ApiKey{
		Owner:     target.ID,
		OwnerType: types.OwnerTypeUser,
		Key:       keyStr,
		Name:      "admin-provisioned",
		Type:      types.APIkeytypeAPI,
	}); err != nil {
		return nil, system.NewHTTPError500("failed to create api key: " + err.Error())
	}
	return &ProvisionUserAPIKeyResponse{UserID: target.ID, APIKey: keyStr}, nil
}
