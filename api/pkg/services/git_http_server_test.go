package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
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
