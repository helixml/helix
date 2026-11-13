package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Project API keys are user-managed keys bound to one project: the
// organization and project are set, no session or spec task. The bearer acts
// as the key's owner, confined to the project by the project-scope checks and
// by route (projectKey in auth_desktop_key.go). Proxy usage made with one is
// attributed to the project.
//
// They are minted with POST /api/v1/api_keys and a project_id, and listed and
// revoked under /api/v1/projects/{id}/api_keys. A caller may manage only its
// own keys, and only with a credential at least as broad as the key: a
// browser session, a personal key, or a key of the project's organization.

var errProjectKeyCaller = errors.New("project API keys can be managed only by signing in or with a personal or organization API key")

// projectKeyManagerRefusal reports why the caller may not mint, list or
// revoke project keys, or nil. A key scoped to a project, spec task, session
// or app is narrower than the keys it would manage.
func projectKeyManagerRefusal(user *types.User) error {
	if !isScopedAPIKeyCaller(user) || isOrgScopedAPIKey(user) {
		return nil
	}
	return errProjectKeyCaller
}

// isOrgScopedAPIKey reports whether the caller holds an ordinary key scoped
// only to an organization.
func isOrgScopedAPIKey(user *types.User) bool {
	return user != nil && user.TokenType == types.TokenTypeAPIKey &&
		user.OrganizationID != "" && user.ProjectID == "" && user.SpecTaskID == "" &&
		user.SessionID == "" && user.AppID == "" &&
		(user.APIKeyType == types.APIkeytypeAPI || user.APIKeyType == types.APIkeytypeNone)
}

// authorizeProjectAPIKeyManagement loads the project a project key is managed
// for and authorizes the caller to it. An organization key reaches only
// projects of its own organization. Callers outside the project's tenant get
// the same 404 as for a project that does not exist.
func (apiServer *HelixAPIServer) authorizeProjectAPIKeyManagement(ctx context.Context, user *types.User, projectID string, action types.Action) (*types.Project, *system.HTTPError) {
	if err := projectKeyManagerRefusal(user); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}
	project, err := apiServer.Store.GetProject(ctx, projectID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, system.NewHTTPError404("project not found")
	}
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	if user.OrganizationID != "" && project.OrganizationID != user.OrganizationID {
		return nil, system.NewHTTPError404("project not found")
	}
	if err := apiServer.authorizeUserToProject(ctx, user, project, action); err != nil {
		if isOutsideTenant(err) {
			return nil, system.NewHTTPError404("project not found")
		}
		log.Debug().Err(err).Str("project_id", projectID).Msg("project API key management refused")
		return nil, system.NewHTTPError403("not authorized to manage API keys for this project")
	}
	return project, nil
}

// createProjectAPIKey mints a key for the caller scoped to the project. The
// owner and organization come from the caller and the project, never the
// request.
func (apiServer *HelixAPIServer) createProjectAPIKey(ctx context.Context, user *types.User, name, projectID string) (*types.ApiKey, error) {
	project, httpErr := apiServer.authorizeProjectAPIKeyManagement(ctx, user, projectID, types.ActionUpdate)
	if httpErr != nil {
		return nil, httpErr
	}
	created, err := apiServer.Controller.CreateAPIKey(ctx, user, &types.ApiKey{
		Name:           name,
		Type:           types.APIkeytypeAPI,
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
	})
	if err != nil {
		return nil, err
	}
	apiServer.auditLogService.LogAPIKeyCreated(ctx, project, created.Summary(), user.ID, user.Email)
	return created, nil
}

// listProjectAPIKeys godoc
// @Summary List project API keys
// @Description List the caller's own API keys scoped to the project. Key secrets are never returned; each key has a non-secret id and key_prefix. Requires a browser session, a personal API key, or an API key of the project's organization.
// @Tags    projects
// @Param   id path string true "Project ID"
// @Success 200 {array} types.APIKeySummary
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/projects/{id}/api_keys [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listProjectAPIKeys(_ http.ResponseWriter, r *http.Request) ([]*types.APIKeySummary, *system.HTTPError) {
	user := getRequestUser(r)
	ctx := r.Context()
	project, httpErr := apiServer.authorizeProjectAPIKeyManagement(ctx, user, mux.Vars(r)["id"], types.ActionGet)
	if httpErr != nil {
		return nil, httpErr
	}
	keys, err := apiServer.listOwnProjectAPIKeys(ctx, user, project)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	summaries := make([]*types.APIKeySummary, 0, len(keys))
	for _, key := range keys {
		summaries = append(summaries, key.Summary())
	}
	return summaries, nil
}

// deleteProjectAPIKey godoc
// @Summary Revoke a project API key
// @Description Revoke one of the caller's own API keys scoped to the project. The key stops authenticating immediately. To rotate a key, create a new one and revoke the old one.
// @Tags    projects
// @Param   id path string true "Project ID"
// @Param   key_id path string true "ID of the API key (the id field from the list)"
// @Success 200 {object} types.APIKeySummary
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/projects/{id}/api_keys/{key_id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteProjectAPIKey(_ http.ResponseWriter, r *http.Request) (*types.APIKeySummary, *system.HTTPError) {
	user := getRequestUser(r)
	ctx := r.Context()
	vars := mux.Vars(r)
	project, httpErr := apiServer.authorizeProjectAPIKeyManagement(ctx, user, vars["id"], types.ActionGet)
	if httpErr != nil {
		return nil, httpErr
	}
	keyID := vars["key_id"]
	if !types.IsAPIKeyID(keyID) {
		return nil, system.NewHTTPError404("API key not found")
	}
	keys, err := apiServer.listOwnProjectAPIKeys(ctx, user, project)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	for _, key := range keys {
		if types.APIKeyID(key.Key) != keyID {
			continue
		}
		if err := apiServer.Store.DeleteAPIKey(ctx, key.Key); err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
		summary := key.Summary()
		apiServer.auditLogService.LogAPIKeyRevoked(ctx, project, summary, user.ID, user.Email)
		return summary, nil
	}
	return nil, system.NewHTTPError404("API key not found")
}

// listOwnProjectAPIKeys returns the caller's user-managed keys for the
// project. Scoping by owner and project happens in the store query.
func (apiServer *HelixAPIServer) listOwnProjectAPIKeys(ctx context.Context, user *types.User, project *types.Project) ([]*types.ApiKey, error) {
	return apiServer.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:          user.ID,
		OwnerType:      user.Type,
		Type:           types.APIkeytypeAPI,
		OrganizationID: project.OrganizationID,
		ProjectID:      project.ID,
		UserManaged:    true,
	})
}
