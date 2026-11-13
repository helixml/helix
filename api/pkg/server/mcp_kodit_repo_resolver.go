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

// resolveKoditRepoScope resolves which Kodit repositories user may search
// through session.
//
// The candidates are the session's tenant: for a session in a project with
// code intelligence on, the project's repositories plus the indexed
// repositories of the project's organization (code search across the
// organization is a product feature); for a helix-org worker's session, which
// has no project, the repositories of the session's organization, or of its
// owner for a personal session. Any other session outside a project (a plain
// chat) has no candidates, so its scope is empty. Each candidate is then kept only if user may read it through the
// REST API, so per-repository access grants apply and a project- or
// session-scoped key never sees a repository its REST calls would be refused.
func resolveKoditRepoScope(ctx context.Context, s store.Store, authorizeRepo RepositoryAuthorizer, session *types.Session, user *types.User) (*koditRepoScope, error) {
	scope := &koditRepoScope{repoIDs: make(map[int64]bool)}
	if authorizeRepo == nil || user == nil {
		return scope, nil
	}

	candidates, err := koditScopeCandidates(ctx, s, session, user)
	if err != nil {
		return nil, err
	}

	for _, repo := range candidates {
		if !repo.KoditIndexing || repo.Metadata == nil {
			continue
		}
		koditID := extractKoditRepoID(repo.Metadata)
		if koditID == 0 {
			log.Debug().
				Str("repo_id", repo.ID).
				Msg("repo has KoditIndexing but no kodit_repo_id, skipping from MCP scope")
			continue
		}
		if scope.repoIDs[koditID] {
			continue
		}
		if err := authorizeRepo(ctx, user, repo, types.ActionGet); err != nil {
			continue
		}
		scope.repoIDs[koditID] = true
		scope.idSlice = append(scope.idSlice, koditID)
	}

	log.Debug().
		Str("session_id", session.ID).
		Str("project_id", sessionProjectID(session)).
		Int("candidate_repos", len(candidates)).
		Int("allowed_repos", len(scope.idSlice)).
		Msg("resolved Kodit repo scope")

	return scope, nil
}

// koditScopeCandidates lists the repositories a session's Kodit scope is
// drawn from, before per-caller authorization.
func koditScopeCandidates(ctx context.Context, s store.Store, session *types.Session, user *types.User) ([]*types.GitRepository, error) {
	projectID := sessionProjectID(session)
	if projectID == "" && session.Metadata.OrgWorkerID == "" {
		return nil, nil
	}
	if projectID == "" {
		query := &types.ListGitRepositoriesRequest{OrganizationID: session.OrganizationID}
		if session.OrganizationID == "" {
			query = &types.ListGitRepositoriesRequest{OwnerID: session.Owner}
		}
		repos, err := s.ListGitRepositories(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("list repos: %w", err)
		}
		return repos, nil
	}

	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("project lookup: %w", err)
	}
	if !project.KoditEnabled {
		return nil, nil
	}

	repos, err := s.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{ProjectID: project.ID})
	if err != nil {
		return nil, fmt.Errorf("list project repos: %w", err)
	}
	// A project-scoped key may only read its own project's repositories, so
	// listing the organization would only add candidates that the
	// authorization step refuses.
	if project.OrganizationID == "" || user.ProjectID != "" {
		return repos, nil
	}
	orgRepos, err := s.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{OrganizationID: project.OrganizationID})
	if err != nil {
		return nil, fmt.Errorf("list org repos: %w", err)
	}
	return append(repos, orgRepos...), nil
}

// sessionProjectID is the project a session belongs to: its project column,
// or else the project recorded in its metadata (as enforceKeySessionScope
// reads it).
func sessionProjectID(session *types.Session) string {
	if session.ProjectID != "" {
		return session.ProjectID
	}
	return session.Metadata.ProjectID
}
