package helix

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
)

// RepoStore is the slice of the Helix store the repository tools need.
// *store.PostgresStore satisfies it structurally.
type RepoStore interface {
	ListGitRepositories(ctx context.Context, request *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error)
	GetGitRepository(ctx context.Context, id string) (*types.GitRepository, error)
	GetProject(ctx context.Context, id string) (*types.Project, error)
	AttachRepositoryToProject(ctx context.Context, projectID, repoID string) error
	DetachRepositoryFromProject(ctx context.Context, projectID, repoID string) error
	SetProjectPrimaryRepository(ctx context.Context, projectID, repoID string) error
}

// Repositories is the helix-runtime implementation of runtime.Repositories.
// Org-scoped list/get, and bot-scoped attach/detach via BotRuntimeState →
// project ID.
type Repositories struct {
	orgStore *store.Store
	helix    RepoStore
}

// NewRepositories builds the impl. Both stores are required.
func NewRepositories(orgStore *store.Store, helix RepoStore) (*Repositories, error) {
	if orgStore == nil {
		return nil, errors.New("helix.NewRepositories: org store is nil")
	}
	if helix == nil {
		return nil, errors.New("helix.NewRepositories: helix store is nil")
	}
	return &Repositories{orgStore: orgStore, helix: helix}, nil
}

var _ runtime.Repositories = (*Repositories)(nil)

func (r *Repositories) List(ctx context.Context, orgID string) ([]runtime.RepoView, error) {
	if orgID == "" {
		return nil, errors.New("orgID is required")
	}
	repos, err := r.helix.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", err)
	}
	return repoViews(repos, ""), nil
}

func (r *Repositories) ListForBot(ctx context.Context, orgID string, botID orgchart.BotID) ([]runtime.RepoView, error) {
	projectID, defaultRepoID, err := r.botProject(ctx, orgID, botID)
	if err != nil {
		return nil, err
	}
	repos, err := r.helix.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		OrganizationID: orgID,
		ProjectID:      projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("list bot repositories: %w", err)
	}
	return repoViews(repos, defaultRepoID), nil
}

func (r *Repositories) AttachToBot(ctx context.Context, orgID string, botID orgchart.BotID, repoID string, primary bool) ([]runtime.RepoView, error) {
	if repoID == "" {
		return nil, errors.New("repo_id is required")
	}
	projectID, _, err := r.botProject(ctx, orgID, botID)
	if err != nil {
		return nil, err
	}
	repo, err := r.helix.GetGitRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("get repository %s: %w", repoID, err)
	}
	if repo.OrganizationID != "" && repo.OrganizationID != orgID {
		return nil, fmt.Errorf("repository %s does not belong to this organization", repoID)
	}
	// Some older rows may lack organization_id; still refuse obvious
	// cross-tenant when set. When empty, project attach is the remaining
	// guard (project is org-scoped via bot state).
	if err := r.helix.AttachRepositoryToProject(ctx, projectID, repoID); err != nil {
		return nil, fmt.Errorf("attach repository: %w", err)
	}
	if primary {
		if err := r.helix.SetProjectPrimaryRepository(ctx, projectID, repoID); err != nil {
			return nil, fmt.Errorf("set primary repository: %w", err)
		}
	}
	return r.ListForBot(ctx, orgID, botID)
}

func (r *Repositories) DetachFromBot(ctx context.Context, orgID string, botID orgchart.BotID, repoID string) ([]runtime.RepoView, error) {
	if repoID == "" {
		return nil, errors.New("repo_id is required")
	}
	projectID, defaultRepoID, err := r.botProject(ctx, orgID, botID)
	if err != nil {
		return nil, err
	}
	if err := r.helix.DetachRepositoryFromProject(ctx, projectID, repoID); err != nil {
		return nil, fmt.Errorf("detach repository: %w", err)
	}
	// If we detached the primary, clear default_repo_id so the project
	// doesn't point at a repo it no longer has.
	if defaultRepoID == repoID {
		if err := r.helix.SetProjectPrimaryRepository(ctx, projectID, ""); err != nil {
			// Best-effort: detach already succeeded; log via returned error
			// only if a later list would be confusing. Prefer soft-fail by
			// re-listing with whatever state remains.
			_ = err
		}
	}
	return r.ListForBot(ctx, orgID, botID)
}

// botProject resolves bot → Helix project id + current default_repo_id.
func (r *Repositories) botProject(ctx context.Context, orgID string, botID orgchart.BotID) (projectID, defaultRepoID string, err error) {
	if orgID == "" {
		return "", "", errors.New("orgID is required")
	}
	if botID == "" {
		return "", "", errors.New("bot_id is required")
	}
	// Confirm the bot exists in this org before touching runtime state.
	if _, err := r.orgStore.Bots.Get(ctx, orgID, botID); err != nil {
		return "", "", fmt.Errorf("get bot %s: %w", botID, err)
	}
	state, err := LoadState(ctx, r.orgStore, orgID, botID)
	if err != nil {
		return "", "", fmt.Errorf("load bot runtime state: %w", err)
	}
	if state.ProjectID == "" {
		return "", "", fmt.Errorf("%w: bot %s", runtime.ErrBotProjectNotReady, botID)
	}
	proj, err := r.helix.GetProject(ctx, state.ProjectID)
	if err != nil {
		// Treat missing/stale project as not-ready so the tool can tell
		// the caller to re-activate the bot.
		return "", "", fmt.Errorf("%w: bot %s project %s: %v", runtime.ErrBotProjectNotReady, botID, state.ProjectID, err)
	}
	if proj.OrganizationID != "" && proj.OrganizationID != orgID {
		return "", "", fmt.Errorf("project %s does not belong to this organization", state.ProjectID)
	}
	return proj.ID, proj.DefaultRepoID, nil
}

func repoViews(repos []*types.GitRepository, primaryID string) []runtime.RepoView {
	out := make([]runtime.RepoView, 0, len(repos))
	for _, repo := range repos {
		if repo == nil {
			continue
		}
		out = append(out, runtime.RepoView{
			ID:            repo.ID,
			Name:          repo.Name,
			Description:   repo.Description,
			CloneURL:      repo.CloneURL,
			ExternalURL:   repo.ExternalURL,
			ExternalType:  string(repo.ExternalType),
			IsExternal:    repo.IsExternal,
			DefaultBranch: repo.DefaultBranch,
			Primary:       primaryID != "" && repo.ID == primaryID,
		})
	}
	return out
}
