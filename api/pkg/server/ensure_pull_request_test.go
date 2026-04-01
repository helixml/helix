package server

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// fakeGitRepoService implements gitRepositoryServicer for tests.
// Methods not expected by a test panic to catch unexpected calls.
type fakeGitRepoService struct {
	listBranchesFunc     func(ctx context.Context, repoID string) ([]string, error)
	getPullRequestFunc   func(ctx context.Context, repoID, id string) (*types.PullRequest, error)
	withRepoLockFunc     func(repoID string, fn func() error) error
	pushBranchFunc       func(ctx context.Context, repoID, branch string, force bool) error
	listPullRequestsFunc func(ctx context.Context, repoID string) ([]*types.PullRequest, error)
	createPRFunc         func(ctx context.Context, repoID, title, desc, src, tgt string) (string, error)
}

func (f *fakeGitRepoService) ListBranches(ctx context.Context, repoID string) ([]string, error) {
	if f.listBranchesFunc != nil {
		return f.listBranchesFunc(ctx, repoID)
	}
	panic("ListBranches called unexpectedly")
}

func (f *fakeGitRepoService) GetPullRequest(ctx context.Context, repoID, id string) (*types.PullRequest, error) {
	if f.getPullRequestFunc != nil {
		return f.getPullRequestFunc(ctx, repoID, id)
	}
	panic("GetPullRequest called unexpectedly")
}

func (f *fakeGitRepoService) WithRepoLock(repoID string, fn func() error) error {
	if f.withRepoLockFunc != nil {
		return f.withRepoLockFunc(repoID, fn)
	}
	panic("WithRepoLock called unexpectedly")
}

func (f *fakeGitRepoService) PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool) error {
	if f.pushBranchFunc != nil {
		return f.pushBranchFunc(ctx, repoID, branchName, force)
	}
	panic("PushBranchToRemote called unexpectedly")
}

func (f *fakeGitRepoService) ListPullRequests(ctx context.Context, repoID string) ([]*types.PullRequest, error) {
	if f.listPullRequestsFunc != nil {
		return f.listPullRequestsFunc(ctx, repoID)
	}
	panic("ListPullRequests called unexpectedly")
}

func (f *fakeGitRepoService) CreatePullRequest(ctx context.Context, repoID, title, desc, src, tgt string) (string, error) {
	if f.createPRFunc != nil {
		return f.createPRFunc(ctx, repoID, title, desc, src, tgt)
	}
	panic("CreatePullRequest called unexpectedly")
}

// Remaining interface methods — not expected in these tests.
func (f *fakeGitRepoService) Initialize(_ context.Context) error { panic("Initialize unexpected") }
func (f *fakeGitRepoService) SetKoditService(_ services.KoditServicer) {
	panic("SetKoditService unexpected")
}
func (f *fakeGitRepoService) CloneRepositoryAsync(_ *types.GitRepository) {
	panic("CloneRepositoryAsync unexpected")
}
func (f *fakeGitRepoService) CreateRepository(_ context.Context, _ *types.GitRepositoryCreateRequest) (*types.GitRepository, error) {
	panic("CreateRepository unexpected")
}
func (f *fakeGitRepoService) GetRepository(_ context.Context, _ string) (*types.GitRepository, error) {
	panic("GetRepository unexpected")
}
func (f *fakeGitRepoService) UpdateRepository(_ context.Context, _ string, _ *types.GitRepositoryUpdateRequest, _ string) (*types.GitRepository, error) {
	panic("UpdateRepository unexpected")
}
func (f *fakeGitRepoService) DeleteRepository(_ context.Context, _ string) error {
	panic("DeleteRepository unexpected")
}
func (f *fakeGitRepoService) CreateSampleRepository(_ context.Context, _ *types.CreateSampleRepositoryRequest) (*types.GitRepository, error) {
	panic("CreateSampleRepository unexpected")
}
func (f *fakeGitRepoService) CreateBranch(_ context.Context, _, _, _ string) error {
	panic("CreateBranch unexpected")
}
func (f *fakeGitRepoService) BrowseTree(_ context.Context, _, _, _ string) ([]types.TreeEntry, error) {
	panic("BrowseTree unexpected")
}
func (f *fakeGitRepoService) GetFileContents(_ context.Context, _, _, _ string) (string, error) {
	panic("GetFileContents unexpected")
}
func (f *fakeGitRepoService) CreateOrUpdateFileContents(_ context.Context, _, _, _ string, _ []byte, _, _, _ string) (string, error) {
	panic("CreateOrUpdateFileContents unexpected")
}
func (f *fakeGitRepoService) GetCloneCommand(_, _ string) string { panic("GetCloneCommand unexpected") }
func (f *fakeGitRepoService) BuildAuthenticatedCloneURL(_, _ string) string {
	panic("BuildAuthenticatedCloneURL unexpected")
}
func (f *fakeGitRepoService) ListCommits(_ context.Context, _ *types.ListCommitsRequest) (*types.ListCommitsResponse, error) {
	panic("ListCommits unexpected")
}
func (f *fakeGitRepoService) PushPullRequest(_ context.Context, _, _ string, _ bool) error {
	panic("PushPullRequest unexpected")
}
func (f *fakeGitRepoService) PullFromRemote(_ context.Context, _, _ string, _ bool) error {
	panic("PullFromRemote unexpected")
}
func (f *fakeGitRepoService) SyncAllBranches(_ context.Context, _ string, _ bool) error {
	panic("SyncAllBranches unexpected")
}
func (f *fakeGitRepoService) GetExternalRepoStatus(_ context.Context, _, _ string) (*types.ExternalStatus, error) {
	panic("GetExternalRepoStatus unexpected")
}
func (f *fakeGitRepoService) GetRepoLock(_ string) *sync.Mutex { panic("GetRepoLock unexpected") }
func (f *fakeGitRepoService) WithExternalRepoRead(_ context.Context, _ *types.GitRepository, _ func() error) error {
	panic("WithExternalRepoRead unexpected")
}
func (f *fakeGitRepoService) WithExternalRepoWrite(_ context.Context, _ *types.GitRepository, _ services.ExternalRepoWriteOptions, _ func() error) error {
	panic("WithExternalRepoWrite unexpected")
}

// TestEnsurePullRequestForRepo_TrackedPRReturnsDirectly verifies that when a task already has a
// tracked RepoPR for the repo, ensurePullRequestForRepo returns that PR immediately without
// calling ListPullRequests or CreatePullRequest.
func TestEnsurePullRequestForRepo_TrackedPRReturnsDirectly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	const (
		repoID   = "repo-1"
		branch   = "feature/task-123"
		prID     = "pr-abc"
		prNumber = 42
		prURL    = "https://github.com/org/repo/pull/42"
	)

	repo := &types.GitRepository{
		ID:            repoID,
		Name:          "myrepo",
		ExternalURL:   "https://github.com/org/repo",
		DefaultBranch: "main",
	}

	task := &types.SpecTask{
		ID:         "task-1",
		BranchName: branch,
		RepoPullRequests: []types.RepoPR{
			{
				RepositoryID: repoID,
				PRID:         prID,
				PRNumber:     prNumber,
				PRURL:        prURL,
				PRState:      "open",
			},
		},
	}

	fake := &fakeGitRepoService{
		// ListBranches must be called first (branch existence check).
		listBranchesFunc: func(_ context.Context, _ string) ([]string, error) {
			return []string{branch}, nil
		},
		// GetPullRequest is called when a tracked PR is found.
		getPullRequestFunc: func(_ context.Context, _ string, id string) (*types.PullRequest, error) {
			require.Equal(t, prID, id)
			return &types.PullRequest{
				ID:     prID,
				Number: prNumber,
				URL:    prURL,
				State:  types.PullRequestStateOpen,
			}, nil
		},
		// ListPullRequests and CreatePullRequest must NOT be called — any call panics.
	}

	server := &HelixAPIServer{
		gitRepositoryService: fake,
	}

	result, err := server.ensurePullRequestForRepo(context.Background(), repo, task, t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, repoID, result.RepositoryID)
	assert.Equal(t, prID, result.PRID)
	assert.Equal(t, prNumber, result.PRNumber)
	assert.Equal(t, prURL, result.PRURL)
}

// TestEnsurePullRequestForRepo_422AlreadyExistsRecovery verifies that when CreatePullRequest
// returns a "already exists" error (race condition between orchestrator and push path),
// the function re-lists PRs and returns the existing one without propagating the error.
func TestEnsurePullRequestForRepo_422AlreadyExistsRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	const (
		repoID   = "repo-2"
		branch   = "feature/task-456"
		prID     = "pr-xyz"
		prNumber = 7
		prURL    = "https://github.com/org/repo/pull/7"
	)

	repo := &types.GitRepository{
		ID:            repoID,
		Name:          "myrepo",
		ExternalURL:   "https://github.com/org/repo",
		DefaultBranch: "main",
	}

	// Task has no tracked PR (so the early-return path is not taken).
	task := &types.SpecTask{
		ID:         "task-2",
		BranchName: branch,
		Name:       "My Task",
		// OrganizationID left empty to avoid Store.GetOrganization call in buildPRFooterForTask.
	}

	listCallCount := 0
	existingPR := &types.PullRequest{
		ID:           prID,
		Number:       prNumber,
		URL:          prURL,
		State:        types.PullRequestStateOpen,
		SourceBranch: branch,
	}

	fake := &fakeGitRepoService{
		listBranchesFunc: func(_ context.Context, _ string) ([]string, error) {
			return []string{branch}, nil
		},
		withRepoLockFunc: func(_ string, fn func() error) error {
			return fn()
		},
		pushBranchFunc: func(_ context.Context, _, _ string, _ bool) error {
			return nil
		},
		listPullRequestsFunc: func(_ context.Context, _ string) ([]*types.PullRequest, error) {
			listCallCount++
			if listCallCount == 1 {
				// First call: no open PRs exist yet.
				return nil, nil
			}
			// Second call (after "already exists" error): return the existing PR.
			return []*types.PullRequest{existingPR}, nil
		},
		createPRFunc: func(_ context.Context, _, _, _, _, _ string) (string, error) {
			return "", errors.New("pull request already exists")
		},
	}

	server := &HelixAPIServer{
		gitRepositoryService: fake,
		Store:                mockStore,
		Cfg:                  &config.ServerConfig{},
	}

	result, err := server.ensurePullRequestForRepo(context.Background(), repo, task, t.TempDir())
	require.NoError(t, err, "should recover from 'already exists' error without propagating it")
	require.NotNil(t, result)
	assert.Equal(t, repoID, result.RepositoryID)
	assert.Equal(t, prID, result.PRID)
	assert.Equal(t, prNumber, result.PRNumber)
	assert.Equal(t, prURL, result.PRURL)
	assert.Equal(t, 2, listCallCount, "ListPullRequests should be called twice: once before create, once after 'already exists'")
}
