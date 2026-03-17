//go:build !nokodit

package server

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// koditRepoScope holds the set of Kodit repositories allowed for a session.
type koditRepoScope struct {
	repoIDs map[int64]bool // allowed Kodit repo IDs (for validation)
	idSlice []int64        // same IDs as a slice (for search filtering)
}

// resolveKoditRepoScope resolves which Kodit repositories a session may access.
// If the session belongs to a project, returns repos linked to that project.
// Otherwise falls back to the user's organization repos.
func resolveKoditRepoScope(ctx context.Context, s store.Store, sessionID string, user *types.User) (*koditRepoScope, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session lookup: %w", err)
	}

	var repos []*types.GitRepository

	switch {
	case session.ProjectID != "":
		project, projErr := s.GetProject(ctx, session.ProjectID)
		if projErr != nil {
			return nil, fmt.Errorf("project lookup: %w", projErr)
		}
		if !project.KoditEnabled {
			return &koditRepoScope{repoIDs: make(map[int64]bool)}, nil
		}
		// When Kodit is enabled, scope to all org repos (not just project repos)
		if project.OrganizationID != "" {
			repos, err = s.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
				OrganizationID: project.OrganizationID,
			})
		} else {
			repos, err = s.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
				OwnerID: user.ID,
			})
		}
	case user.OrganizationID != "":
		repos, err = s.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			OrganizationID: user.OrganizationID,
		})
	default:
		repos, err = s.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			OwnerID: user.ID,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}

	scope := &koditRepoScope{
		repoIDs: make(map[int64]bool),
	}

	for _, repo := range repos {
		if !repo.KoditIndexing {
			continue
		}
		if repo.Metadata == nil {
			continue
		}
		koditID := extractKoditRepoID(repo.Metadata)
		if koditID == 0 {
			log.Debug().
				Str("repo_id", repo.ID).
				Msg("repo has KoditIndexing but no kodit_repo_id, skipping from MCP scope")
			continue
		}
		scope.repoIDs[koditID] = true
		scope.idSlice = append(scope.idSlice, koditID)
	}

	log.Debug().
		Str("session_id", sessionID).
		Str("project_id", session.ProjectID).
		Int("allowed_repos", len(scope.idSlice)).
		Msg("resolved Kodit repo scope")

	return scope, nil
}
