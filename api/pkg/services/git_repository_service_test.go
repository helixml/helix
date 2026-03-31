package services

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
)

// fakeStore embeds store.Store and overrides only the methods we need.
type fakeStore struct {
	store.Store
	repo               *types.GitRepository
	deleted            bool
	koditRepoRefCount  int64
}

func (f *fakeStore) GetGitRepository(_ context.Context, _ string) (*types.GitRepository, error) {
	if f.repo == nil {
		return nil, store.ErrNotFound
	}
	return f.repo, nil
}

func (f *fakeStore) DeleteGitRepository(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

func (f *fakeStore) CountGitRepositoriesByKoditRepoID(_ context.Context, _ int64, _ string) (int64, error) {
	return f.koditRepoRefCount, nil
}

// fakeKodit records calls for verification.
type fakeKodit struct {
	enabled          bool
	deletedRepoID    int64
	deleteRepoCalled bool
	err              error
}

func (f *fakeKodit) IsEnabled() bool          { return f.enabled }
func (f *fakeKodit) MCPDocumentation() string { return "" }
func (f *fakeKodit) RegisterRepository(_ context.Context, _ *RegisterRepositoryParams) (int64, bool, error) {
	return 0, false, f.err
}
func (f *fakeKodit) DeleteRepository(_ context.Context, id int64) error {
	f.deleteRepoCalled = true
	f.deletedRepoID = id
	return f.err
}
func (f *fakeKodit) GetRepositoryEnrichments(_ context.Context, _ int64, _, _ string) ([]enrichment.Enrichment, error) {
	return nil, f.err
}
func (f *fakeKodit) GetEnrichment(_ context.Context, _ string) (enrichment.Enrichment, error) {
	return enrichment.Enrichment{}, f.err
}
func (f *fakeKodit) GetRepositoryCommits(_ context.Context, _ int64, _ int) ([]repository.Commit, error) {
	return nil, f.err
}
func (f *fakeKodit) SearchSnippets(_ context.Context, _ int64, _ string, _ int) ([]enrichment.Enrichment, error) {
	return nil, f.err
}
func (f *fakeKodit) GetRepositoryStatus(_ context.Context, _ int64) (tracking.RepositoryStatusSummary, error) {
	return tracking.RepositoryStatusSummary{}, f.err
}
func (f *fakeKodit) RescanCommit(_ context.Context, _ int64, _ string) error { return f.err }
func (f *fakeKodit) ListRepositories(_ context.Context, _, _ int) ([]repository.Repository, int64, error) {
	return nil, 0, f.err
}
func (f *fakeKodit) RepositorySummary(_ context.Context, _ int64) (repository.RepositorySummary, error) {
	return repository.RepositorySummary{}, f.err
}
func (f *fakeKodit) SyncRepository(_ context.Context, _ int64) error { return f.err }
func (f *fakeKodit) EnrichmentCount(_ context.Context, _ int64) (int64, error) {
	return 0, f.err
}
func (f *fakeKodit) SystemStats(_ context.Context) (KoditSystemStats, error) {
	return KoditSystemStats{}, f.err
}
func (f *fakeKodit) RepositoryTasks(_ context.Context, _ int64) (KoditRepositoryTasks, error) {
	return KoditRepositoryTasks{}, f.err
}
func (f *fakeKodit) ListAllTasks(_ context.Context, _, _ int) ([]KoditPendingTask, int64, error) {
	return nil, 0, f.err
}
func (f *fakeKodit) ActiveTasks(_ context.Context) ([]KoditActiveTask, error) {
	return nil, f.err
}
func (f *fakeKodit) DeleteTask(_ context.Context, _ int64) error { return f.err }
func (f *fakeKodit) UpdateTaskPriority(_ context.Context, _ int64, _ int) error { return f.err }
func (f *fakeKodit) GetWikiTree(_ context.Context, _ int64) ([]KoditWikiTreeNode, error) {
	return nil, f.err
}
func (f *fakeKodit) GetWikiPage(_ context.Context, _ int64, _ string) (*KoditWikiPage, error) {
	return nil, f.err
}
func (f *fakeKodit) SemanticSearch(_ context.Context, _ int64, _ string, _ int, _ string) ([]KoditFileResult, error) {
	return nil, f.err
}
func (f *fakeKodit) KeywordSearch(_ context.Context, _ int64, _ string, _ int, _ string) ([]KoditFileResult, error) {
	return nil, f.err
}
func (f *fakeKodit) GrepSearch(_ context.Context, _ int64, _ string, _ string, _ int) ([]KoditGrepResult, error) {
	return nil, f.err
}
func (f *fakeKodit) ListFiles(_ context.Context, _ int64, _ string) ([]KoditFileEntry, error) {
	return nil, f.err
}
func (f *fakeKodit) ReadFile(_ context.Context, _ int64, _ string, _, _ int) (*KoditFileContent, error) {
	return nil, f.err
}
func (f *fakeKodit) UpdateChunkingConfig(_ context.Context, _ int64, _, _, _ int) error {
	return f.err
}

func TestDeleteRepository_DeletesFromKodit(t *testing.T) {
	kodit := &fakeKodit{enabled: true}
	st := &fakeStore{
		repo: &types.GitRepository{
			ID:            "repo-1",
			KoditIndexing: true,
			Metadata:      map[string]any{"kodit_repo_id": int64(42)},
		},
	}
	svc := NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@test.com")
	svc.SetKoditService(kodit)

	if err := svc.DeleteRepository(t.Context(), "repo-1"); err != nil {
		t.Fatalf("DeleteRepository() error: %v", err)
	}

	if !kodit.deleteRepoCalled {
		t.Error("expected kodit DeleteRepository to be called")
	}
	if kodit.deletedRepoID != 42 {
		t.Errorf("expected kodit repo ID 42, got %d", kodit.deletedRepoID)
	}
	if !st.deleted {
		t.Error("expected store DeleteGitRepository to be called")
	}
}

func TestDeleteRepository_SkipsKoditWhenShared(t *testing.T) {
	kodit := &fakeKodit{enabled: true}
	st := &fakeStore{
		repo: &types.GitRepository{
			ID:            "repo-1",
			KoditIndexing: true,
			Metadata:      map[string]any{"kodit_repo_id": int64(42)},
		},
		koditRepoRefCount: 1, // another repo shares this kodit index
	}
	svc := NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@test.com")
	svc.SetKoditService(kodit)

	if err := svc.DeleteRepository(t.Context(), "repo-1"); err != nil {
		t.Fatalf("DeleteRepository() error: %v", err)
	}

	if kodit.deleteRepoCalled {
		t.Error("expected kodit DeleteRepository NOT to be called when other repos share the index")
	}
	if !st.deleted {
		t.Error("expected store DeleteGitRepository to be called")
	}
}

func TestDeleteRepository_SkipsKoditWhenNoRepoID(t *testing.T) {
	kodit := &fakeKodit{enabled: true}
	st := &fakeStore{
		repo: &types.GitRepository{
			ID:            "repo-2",
			KoditIndexing: true,
			Metadata:      map[string]any{},
		},
	}
	svc := NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@test.com")
	svc.SetKoditService(kodit)

	if err := svc.DeleteRepository(t.Context(), "repo-2"); err != nil {
		t.Fatalf("DeleteRepository() error: %v", err)
	}

	if kodit.deleteRepoCalled {
		t.Error("expected kodit DeleteRepository NOT to be called when no kodit_repo_id")
	}
}

func TestDeleteRepository_SkipsKoditWhenDisabled(t *testing.T) {
	kodit := &fakeKodit{enabled: false}
	st := &fakeStore{
		repo: &types.GitRepository{
			ID:            "repo-3",
			KoditIndexing: true,
			Metadata:      map[string]any{"kodit_repo_id": int64(42)},
		},
	}
	svc := NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@test.com")
	svc.SetKoditService(kodit)

	if err := svc.DeleteRepository(t.Context(), "repo-3"); err != nil {
		t.Fatalf("DeleteRepository() error: %v", err)
	}

	if kodit.deleteRepoCalled {
		t.Error("expected kodit DeleteRepository NOT to be called when kodit is disabled")
	}
}

func TestGetPullRequestURL(t *testing.T) {
	tests := []struct {
		name          string
		repo          *types.GitRepository
		pullRequestID string
		expected      string
	}{
		{
			name: "GitHub repo without .git suffix",
			repo: &types.GitRepository{
				ExternalURL:  "https://github.com/chocobar/demo-recipes",
				ExternalType: types.ExternalRepositoryTypeGitHub,
			},
			pullRequestID: "1",
			expected:      "https://github.com/chocobar/demo-recipes/pull/1",
		},
		{
			name: "GitHub repo with .git suffix",
			repo: &types.GitRepository{
				ExternalURL:  "https://github.com/chocobar/demo-recipes.git",
				ExternalType: types.ExternalRepositoryTypeGitHub,
			},
			pullRequestID: "1",
			expected:      "https://github.com/chocobar/demo-recipes/pull/1",
		},
		{
			name: "Azure DevOps repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://dev.azure.com/org/project/_git/repo",
				ExternalType: types.ExternalRepositoryTypeADO,
			},
			pullRequestID: "42",
			expected:      "https://dev.azure.com/org/project/_git/repo/pullrequest/42",
		},
		{
			name: "GitLab repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://gitlab.com/org/repo",
				ExternalType: types.ExternalRepositoryTypeGitLab,
			},
			pullRequestID: "123",
			expected:      "https://gitlab.com/org/repo/merge_requests/123",
		},
		{
			name: "Bitbucket repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://bitbucket.org/org/repo",
				ExternalType: types.ExternalRepositoryTypeBitbucket,
			},
			pullRequestID: "99",
			expected:      "https://bitbucket.org/org/repo/pull-requests/99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPullRequestURL(tt.repo, tt.pullRequestID)
			if result != tt.expected {
				t.Errorf("GetPullRequestURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}
