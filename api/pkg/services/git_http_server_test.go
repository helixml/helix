package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type HasReadAccessSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockStore   *store.MockStore
	repoService *GitRepositoryService
	tmpDir      string
}

func TestHasReadAccessSuite(t *testing.T) {
	suite.Run(t, new(HasReadAccessSuite))
}

func (s *HasReadAccessSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)

	// updateRepositoryFromGit calls UpdateGitRepository after reading repo info
	s.mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	tmpDir, err := os.MkdirTemp("", "git-http-server-test-*")
	s.Require().NoError(err)
	s.tmpDir = tmpDir

	s.repoService = &GitRepositoryService{
		store:       s.mockStore,
		gitRepoBase: filepath.Join(tmpDir, "repos"),
	}
}

func (s *HasReadAccessSuite) TearDownTest() {
	os.RemoveAll(s.tmpDir)
}

// createBareRepo creates a bare git repo on disk and returns its path.
func (s *HasReadAccessSuite) createBareRepo(name string) string {
	repoPath := filepath.Join(s.tmpDir, name+".git")
	cmd := exec.Command("git", "init", "--bare", repoPath)
	out, err := cmd.CombinedOutput()
	s.Require().NoError(err, "git init --bare failed: %s", string(out))
	return repoPath
}

func (s *HasReadAccessSuite) newServer(authFn AuthorizationToRepositoryFunc) *GitHTTPServer {
	return &GitHTTPServer{
		store:          s.mockStore,
		gitRepoService: s.repoService,
		authorizeFn:    authFn,
	}
}

func (s *HasReadAccessSuite) TestNoAuthorizeFn_AllowsAccess() {
	server := s.newServer(nil)
	user := &types.User{ID: "user1"}

	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.True(result)
}

func (s *HasReadAccessSuite) TestRepoNotFound_DeniesAccess() {
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return nil
	}
	server := s.newServer(authFn)

	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(nil, fmt.Errorf("not found"))

	user := &types.User{ID: "user1"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.False(result)
}

func (s *HasReadAccessSuite) TestOwner_AllowsAccess() {
	repoPath := s.createBareRepo("repo1")

	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return fmt.Errorf("should not be called for owner")
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:        "repo1",
		OwnerID:   "user1",
		LocalPath: repoPath,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo1").Return(repo, nil)

	user := &types.User{ID: "user1"}
	result := server.hasReadAccess(context.Background(), user, "repo1")
	s.True(result)
}

func (s *HasReadAccessSuite) TestNonOwner_AuthorizeFnAllows() {
	repoPath := s.createBareRepo("repo2")

	var capturedAction types.Action
	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		capturedAction = action
		return nil
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:        "repo2",
		OwnerID:   "owner1",
		LocalPath: repoPath,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo2").Return(repo, nil)

	user := &types.User{ID: "user2"}
	result := server.hasReadAccess(context.Background(), user, "repo2")
	s.True(result)
	s.Equal(types.ActionGet, capturedAction, "hasReadAccess should use ActionGet")
}

func (s *HasReadAccessSuite) TestNonOwner_AuthorizeFnDenies() {
	repoPath := s.createBareRepo("repo3")

	authFn := func(ctx context.Context, user *types.User, repo *types.GitRepository, action types.Action) error {
		return fmt.Errorf("not authorized")
	}
	server := s.newServer(authFn)

	repo := &types.GitRepository{
		ID:        "repo3",
		OwnerID:   "owner1",
		LocalPath: repoPath,
	}
	s.mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo3").Return(repo, nil)

	user := &types.User{ID: "user2"}
	result := server.hasReadAccess(context.Background(), user, "repo3")
	s.False(result)
}

func TestDetectFollowUpPullRequestReady_MarksNewSecondaryRepoWithChanges(t *testing.T) {
	repoPath, remotePath := createFollowUpTestRepo(t, true)
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "task-1",
		ProjectID:  "project-1",
		BranchName: "feature/follow-up",
		Status:     types.TaskStatusDone,
		RepoPullRequests: []types.RepoPR{
			{RepositoryID: "repo-1", PRID: "67", PRState: "merged"},
		},
	}
	repo := &types.GitRepository{ID: "repo-2", ExternalURL: remotePath, IsExternal: true, LocalPath: repoPath, DefaultBranch: "main"}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{ID: task.ProjectID, DefaultRepoID: "repo-1"}, nil)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil).AnyTimes()
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, updated *types.SpecTask) error {
		assert.Equal(t, types.TaskStatusDone, updated.Status)
		assert.Equal(t, []types.RepoPR{
			{RepositoryID: "repo-1", PRID: "67", PRState: "merged"},
		}, updated.RepoPullRequests)
		assert.Equal(t, map[string]bool{"repo-2": true}, updated.Metadata[FollowUpPRReadyMetadataKey])
		return nil
	})
	server := &GitHTTPServer{
		store: mockStore,
		gitRepoService: &GitRepositoryService{
			store:     mockStore,
			repoLocks: make(map[string]*sync.Mutex),
		},
	}

	require.NoError(t, server.detectFollowUpPullRequestReady(context.Background(), repo, task.ID, task.BranchName, repoPath))
}

func TestDetectFollowUpPullRequestReady_NoDiffClearsRepositoryReadiness(t *testing.T) {
	repoPath, remotePath := createFollowUpTestRepo(t, false)
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:         "task-1",
		ProjectID:  "project-1",
		BranchName: "feature/follow-up",
		Status:     types.TaskStatusDone,
		Metadata: map[string]interface{}{
			FollowUpPRReadyMetadataKey: map[string]interface{}{"repo-1": true},
		},
		RepoPullRequests: []types.RepoPR{
			{RepositoryID: "repo-1", PRID: "67", PRState: "merged"},
		},
	}
	repo := &types.GitRepository{ID: "repo-1", ExternalURL: remotePath, IsExternal: true, LocalPath: repoPath, DefaultBranch: "main"}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{ID: task.ProjectID, DefaultRepoID: repo.ID}, nil)
	mockStore.EXPECT().GetGitRepository(gomock.Any(), repo.ID).Return(repo, nil).AnyTimes()
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, updated *types.SpecTask) error {
		_, exists := updated.Metadata[FollowUpPRReadyMetadataKey]
		assert.False(t, exists)
		return nil
	})
	server := &GitHTTPServer{
		store: mockStore,
		gitRepoService: &GitRepositoryService{
			store:     mockStore,
			repoLocks: make(map[string]*sync.Mutex),
		},
	}

	require.NoError(t, server.detectFollowUpPullRequestReady(context.Background(), repo, task.ID, task.BranchName, repoPath))
	assert.NotContains(t, task.Metadata, FollowUpPRReadyMetadataKey)
	assert.Equal(t, types.TaskStatusDone, task.Status)
}

func TestSetFollowUpPRReady_PreservesOtherRepositories(t *testing.T) {
	metadata := map[string]interface{}{
		FollowUpPRReadyMetadataKey: map[string]interface{}{
			"repo-1": true,
			"repo-2": true,
		},
	}

	assert.True(t, setFollowUpPRReady(metadata, "repo-1", false))
	assert.Equal(t, map[string]bool{"repo-2": true}, metadata[FollowUpPRReadyMetadataKey])
}

func createFollowUpTestRepo(t *testing.T, changed bool) (string, string) {
	t.Helper()
	root := t.TempDir()
	workPath := filepath.Join(root, "work")
	require.NoError(t, os.Mkdir(workPath, 0o700))
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	run(workPath, "init", "-b", "main")
	run(workPath, "config", "user.email", "test@example.com")
	run(workPath, "config", "user.name", "Test User")
	require.NoError(t, os.WriteFile(filepath.Join(workPath, "file.txt"), []byte("base\n"), 0o600))
	run(workPath, "add", "file.txt")
	run(workPath, "commit", "-m", "base")
	run(workPath, "checkout", "-b", "feature/follow-up")
	if changed {
		require.NoError(t, os.WriteFile(filepath.Join(workPath, "file.txt"), []byte("follow-up\n"), 0o600))
		run(workPath, "add", "file.txt")
		run(workPath, "commit", "-m", "follow-up")
	} else {
		run(workPath, "commit", "--allow-empty", "-m", "merge-only")
	}
	remotePath := filepath.Join(root, "remote.git")
	repoPath := filepath.Join(root, "local.git")
	run(root, "clone", "--bare", workPath, remotePath)
	run(root, "clone", "--bare", workPath, repoPath)
	return repoPath, remotePath
}

// TestParsePullRequestMarkdown tests the parsePullRequestMarkdown function
func TestParsePullRequestMarkdown(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantTitle string
		wantDesc  string
		wantOK    bool
	}{
		{
			name:      "empty content",
			content:   "",
			wantTitle: "",
			wantDesc:  "",
			wantOK:    false,
		},
		{
			name:      "whitespace only",
			content:   "   \n\t\n  ",
			wantTitle: "",
			wantDesc:  "",
			wantOK:    false,
		},
		{
			name:      "title only with hash",
			content:   "# My PR Title",
			wantTitle: "My PR Title",
			wantDesc:  "",
			wantOK:    true,
		},
		{
			name:      "title only without hash",
			content:   "My PR Title",
			wantTitle: "My PR Title",
			wantDesc:  "",
			wantOK:    true,
		},
		{
			name:      "title with description",
			content:   "# Add new feature\n\nThis PR adds a cool new feature.",
			wantTitle: "Add new feature",
			wantDesc:  "This PR adds a cool new feature.",
			wantOK:    true,
		},
		{
			name: "full PR with sections",
			content: `# Fix authentication bug

## Summary
Fixed a bug where users couldn't log in.

## Changes
- Updated auth middleware
- Fixed token validation`,
			wantTitle: "Fix authentication bug",
			wantDesc: `## Summary
Fixed a bug where users couldn't log in.

## Changes
- Updated auth middleware
- Fixed token validation`,
			wantOK: true,
		},
		{
			name:      "multiple blank lines before description",
			content:   "# Title\n\n\n\nDescription here",
			wantTitle: "Title",
			wantDesc:  "Description here",
			wantOK:    true,
		},
		{
			name:      "hash in title only strips once",
			content:   "# # Double hash title",
			wantTitle: "# Double hash title",
			wantDesc:  "",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, desc, ok := parsePullRequestMarkdown(tt.content)
			assert.Equal(t, tt.wantOK, ok, "ok mismatch")
			assert.Equal(t, tt.wantTitle, title, "title mismatch")
			assert.Equal(t, tt.wantDesc, desc, "description mismatch")
		})
	}
}

// TestGetSpecDocsBaseURL tests URL generation for different repo types
func TestGetSpecDocsBaseURL(t *testing.T) {
	tests := []struct {
		name          string
		repo          *types.GitRepository
		designDocPath string
		want          string
	}{
		{
			name: "GitHub repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://github.com/org/repo",
				ExternalType: types.ExternalRepositoryTypeGitHub,
			},
			designDocPath: "001234_my-task",
			want:          "https://github.com/org/repo/blob/helix-specs/design/tasks/001234_my-task",
		},
		{
			name: "GitHub repo with .git suffix",
			repo: &types.GitRepository{
				ExternalURL:  "https://github.com/org/repo.git",
				ExternalType: types.ExternalRepositoryTypeGitHub,
			},
			designDocPath: "001234_my-task",
			want:          "https://github.com/org/repo/blob/helix-specs/design/tasks/001234_my-task",
		},
		{
			name: "GitLab repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://gitlab.com/org/repo",
				ExternalType: types.ExternalRepositoryTypeGitLab,
			},
			designDocPath: "001234_my-task",
			want:          "https://gitlab.com/org/repo/-/blob/helix-specs/design/tasks/001234_my-task",
		},
		{
			name: "Azure DevOps repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://dev.azure.com/org/project/_git/repo",
				ExternalType: types.ExternalRepositoryTypeADO,
			},
			designDocPath: "001234_my-task",
			want:          "https://dev.azure.com/org/project/_git/repo?path=/design/tasks/001234_my-task&version=GBhelix-specs",
		},
		{
			name: "Bitbucket repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://bitbucket.org/org/repo",
				ExternalType: types.ExternalRepositoryTypeBitbucket,
			},
			designDocPath: "001234_my-task",
			want:          "https://bitbucket.org/org/repo/src/helix-specs/design/tasks/001234_my-task",
		},
		{
			name: "Internal repo (no external URL)",
			repo: &types.GitRepository{
				ExternalURL: "",
			},
			designDocPath: "001234_my-task",
			want:          "",
		},
		{
			name: "Unknown repo type",
			repo: &types.GitRepository{
				ExternalURL:  "https://unknown.com/repo",
				ExternalType: "unknown",
			},
			designDocPath: "001234_my-task",
			want:          "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSpecDocsBaseURL(tt.repo, tt.designDocPath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPullRequestFileChangedForTask(t *testing.T) {
	tests := []struct {
		name          string
		files         []string
		designDocPath string
		want          bool
	}{
		{
			name:          "empty designDocPath returns false",
			files:         []string{"design/tasks/spt_01abc/pull_request_helix.md"},
			designDocPath: "",
			want:          false,
		},
		{
			name:          "matching pull_request_<repo>.md",
			files:         []string{"design/tasks/spt_01abc/pull_request_helix.md"},
			designDocPath: "spt_01abc",
			want:          true,
		},
		{
			name:          "matching generic pull_request.md",
			files:         []string{"design/tasks/spt_01abc/pull_request.md"},
			designDocPath: "spt_01abc",
			want:          true,
		},
		{
			name:          "matching pull_request_<repo-with-dashes>.md",
			files:         []string{"design/tasks/spt_01abc/pull_request_qwen-code.md"},
			designDocPath: "spt_01abc",
			want:          true,
		},
		{
			name:          "wrong task dir ignored",
			files:         []string{"design/tasks/spt_other/pull_request_helix.md"},
			designDocPath: "spt_01abc",
			want:          false,
		},
		{
			name:          "non-pull_request file in same dir ignored",
			files:         []string{"design/tasks/spt_01abc/requirements.md", "design/tasks/spt_01abc/design.md"},
			designDocPath: "spt_01abc",
			want:          false,
		},
		{
			name:          "file in subdirectory (screenshots) ignored",
			files:         []string{"design/tasks/spt_01abc/screenshots/pull_request_helix.md"},
			designDocPath: "spt_01abc",
			want:          false,
		},
		{
			name:          "non-md file ignored",
			files:         []string{"design/tasks/spt_01abc/pull_request_helix.txt"},
			designDocPath: "spt_01abc",
			want:          false,
		},
		{
			name:          "mix returns true when at least one PR file present",
			files:         []string{"design/tasks/spt_01abc/requirements.md", "design/tasks/spt_01abc/pull_request_helix.md"},
			designDocPath: "spt_01abc",
			want:          true,
		},
		{
			name:          "empty files list",
			files:         nil,
			designDocPath: "spt_01abc",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pullRequestFileChangedForTask(tt.files, tt.designDocPath)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestOpenPRDescriptionTargetsIncludesEveryOpenExternalRepo(t *testing.T) {
	repos := []*types.GitRepository{
		{ID: "repo-primary", Name: "helix", ExternalURL: "https://github.com/helixml/helix"},
		{ID: "repo-secondary", Name: "zed", ExternalURL: "https://github.com/helixml/zed"},
		{ID: "repo-closed", Name: "closed", ExternalURL: "https://github.com/helixml/closed"},
		{ID: "repo-internal", Name: "internal"},
	}
	task := &types.SpecTask{RepoPullRequests: []types.RepoPR{
		{RepositoryID: "repo-primary", PRNumber: 101, PRState: "open"},
		{RepositoryID: "repo-secondary", PRNumber: 202, PRState: "open"},
		{RepositoryID: "repo-closed", PRNumber: 303, PRState: "closed"},
		{RepositoryID: "repo-internal", PRNumber: 404, PRState: "open"},
	}}

	targets := openPRDescriptionTargets(task, repos)
	require.Len(t, targets, 2)
	assert.Equal(t, "repo-primary", targets[0].repository.ID)
	assert.Equal(t, 101, targets[0].pullRequest.PRNumber)
	assert.Equal(t, "repo-secondary", targets[1].repository.ID)
	assert.Equal(t, 202, targets[1].pullRequest.PRNumber)
}
