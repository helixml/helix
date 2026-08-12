package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/vcs"
	"github.com/rs/zerolog/log"
)

// getProjectVCSConnections godoc
// @Summary Get project VCS connection status
// @Description One entry per distinct VCS provider present among the project's external repos, with the acting user, the account pushes are attributed to, and per-repo verified access. Backs the project board connection lozenge.
// @Tags Projects
// @ID getProjectVCSConnections
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} types.VCSConnectionInfo
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/vcs-connections [get]
func (s *HelixAPIServer) getProjectVCSConnections(_ http.ResponseWriter, r *http.Request) ([]types.VCSConnectionInfo, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}
	if err := s.authorizeUserToProject(r.Context(), user, project, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	repos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{ProjectID: projectID})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Presence axis: one lozenge per distinct provider present among the
	// project's external repos. Local-only repos contribute nothing.
	reposByProvider := map[types.ExternalRepositoryType][]*types.GitRepository{}
	order := []types.ExternalRepositoryType{}
	for _, repo := range repos {
		if !repo.IsExternal || repo.ExternalType == "" {
			continue
		}
		if _, seen := reposByProvider[repo.ExternalType]; !seen {
			order = append(order, repo.ExternalType)
		}
		reposByProvider[repo.ExternalType] = append(reposByProvider[repo.ExternalType], repo)
	}

	conns, err := s.Store.ListOAuthConnections(r.Context(), &store.ListOAuthConnectionsQuery{UserID: user.ID})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	acting := types.VCSActingUser{ID: user.ID, Name: userDisplayName(user)}

	result := []types.VCSConnectionInfo{}
	for _, providerType := range order {
		info := types.VCSConnectionInfo{
			Provider:      providerType,
			ActingUser:    acting,
			Repos:         []types.VCSRepoAccess{},
			MissingScopes: []string{},
		}

		conn := findConnectionForProvider(conns, providerType)
		if conn == nil {
			info.State = types.VCSConnectionDisconnected
			result = append(result, info)
			continue
		}

		info.PushingAs = &types.VCSPushingAs{
			Username:     vcs.IdentityHandle(conn),
			ConnectionID: conn.ID,
		}
		info.MissingScopes = getMissingScopes(conn.Scopes, vcs.RequiredScopes(providerType))

		// State axis: verified unless a repo probe definitively reports no access.
		allAccessible := true
		for _, repo := range reposByProvider[providerType] {
			ownerRepo := services.RepoOwnerName(repo)
			hasAccess, verified := s.verifyVCSRepoAccess(r.Context(), conn, providerType, ownerRepo)
			if verified && !hasAccess {
				allAccessible = false
			}
			info.Repos = append(info.Repos, types.VCSRepoAccess{
				Repo:      ownerRepo,
				HasAccess: hasAccess,
				Verified:  verified,
			})
		}

		if allAccessible {
			info.State = types.VCSConnectionVerified
		} else {
			info.State = types.VCSConnectionNeedsAttention
		}
		result = append(result, info)
	}

	return result, nil
}

// verifyVCSRepoAccess probes the provider to check the connection can reach
// owner/repo. Returns (hasAccess, verified). verified=false means we couldn't
// definitively check (no probe for the provider, or the request couldn't be made)
// — in that case hasAccess is reported optimistically as true so we don't false-
// alarm. Only a real non-2xx response (e.g. GitHub's misleading 404 for a private
// repo the account can't see) marks the repo inaccessible.
func (s *HelixAPIServer) verifyVCSRepoAccess(ctx context.Context, conn *types.OAuthConnection, externalType types.ExternalRepositoryType, ownerRepo string) (hasAccess, verified bool) {
	parts := strings.SplitN(ownerRepo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return true, false
	}
	probeURL := vcs.AccessProbeURL(externalType, parts[0], parts[1])
	if probeURL == "" {
		return true, false // no probe for this provider — can't disprove access
	}
	provider, err := s.oauthManager.GetProvider(conn.ProviderID)
	if err != nil {
		log.Warn().Err(err).Str("connection_id", conn.ID).Msg("VCS verify: could not load provider")
		return true, false
	}
	resp, err := provider.MakeAuthorizedRequest(ctx, conn, http.MethodGet, probeURL, nil)
	if err != nil {
		log.Warn().Err(err).Str("repo", ownerRepo).Msg("VCS verify: probe request failed (unverifiable)")
		return true, false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300, true
}

// findConnectionForProvider returns the acting user's connection for a repo
// provider type, mirroring the acting-user-first credential resolution.
func findConnectionForProvider(conns []*types.OAuthConnection, externalType types.ExternalRepositoryType) *types.OAuthConnection {
	want := services.OAuthProviderTypeForRepo(externalType)
	for _, conn := range conns {
		if conn.Provider.Type == want && conn.AccessToken != "" {
			return conn
		}
	}
	return nil
}

func userDisplayName(u *types.User) string {
	if u.FullName != "" {
		return u.FullName
	}
	if u.Username != "" {
		return u.Username
	}
	return u.Email
}
