package server

import (
	"context"
	"sync"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
)

// gitRepositoryServicer is the interface used by HelixAPIServer for git repository operations.
// The concrete implementation is *services.GitRepositoryService.
type gitRepositoryServicer interface {
	Initialize(ctx context.Context) error
	SetKoditService(koditService services.KoditServicer)
	CloneRepositoryAsync(gitRepo *types.GitRepository)
	CreateRepository(ctx context.Context, request *types.GitRepositoryCreateRequest) (*types.GitRepository, error)
	GetRepository(ctx context.Context, repoID string) (*types.GitRepository, error)
	UpdateRepository(ctx context.Context, repoID string, request *types.GitRepositoryUpdateRequest, koditAPIKey string) (*types.GitRepository, error)
	DeleteRepository(ctx context.Context, repoID string) error
	CreateSampleRepository(ctx context.Context, request *types.CreateSampleRepositoryRequest) (*types.GitRepository, error)
	CreateBranch(ctx context.Context, repoID, branchName, baseBranch string) error
	ListBranches(ctx context.Context, repoID string) ([]string, error)
	BrowseTree(ctx context.Context, repoID, path, branch string) ([]types.TreeEntry, error)
	GetFileContents(ctx context.Context, repoID, path, branch string) (string, error)
	CreateOrUpdateFileContents(ctx context.Context, repoID, path, branch string, content []byte, commitMessage, authorName, authorEmail string) (string, error)
	GetCloneCommand(repoID, targetDir string) string
	BuildAuthenticatedCloneURL(repoID, apiKey string) string
	ListCommits(ctx context.Context, req *types.ListCommitsRequest) (*types.ListCommitsResponse, error)
	CreatePullRequest(ctx context.Context, repoID, title, description, sourceBranch, targetBranch string) (string, error)
	GetPullRequest(ctx context.Context, repoID, id string) (*types.PullRequest, error)
	ListPullRequests(ctx context.Context, repoID string) ([]*types.PullRequest, error)
	PushPullRequest(ctx context.Context, repoID, branchName string, force bool) error
	PullFromRemote(ctx context.Context, repoID, branchName string, force bool) error
	PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool) error
	SyncAllBranches(ctx context.Context, repoID string, force bool) error
	GetExternalRepoStatus(ctx context.Context, repoID, branchName string) (*types.ExternalStatus, error)
	GetRepoLock(repoID string) *sync.Mutex
	WithRepoLock(repoID string, fn func() error) error
	WithExternalRepoRead(ctx context.Context, repo *types.GitRepository, readFunc func() error) error
	WithExternalRepoWrite(ctx context.Context, repo *types.GitRepository, opts services.ExternalRepoWriteOptions, writeFunc func() error) error
}
