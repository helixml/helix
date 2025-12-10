package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type GitRepositoryPushSuiteADO struct {
	suite.Suite
	store *store.MockStore
	s     *GitRepositoryService

	adoToken string
	adoRepo  string
	testDir  string
}

func TestGitRepositoryPushSuiteADO(t *testing.T) {
	suite.Run(t, new(GitRepositoryPushSuiteADO))
}

func (suite *GitRepositoryPushSuiteADO) SetupTest() {
	suite.store = store.NewMockStore(gomock.NewController(suite.T()))
	suite.adoToken = os.Getenv("CI_ADO_TOKEN")
	suite.adoRepo = os.Getenv("CI_ADO_REPO")
	suite.testDir = suite.T().TempDir()
	suite.s = NewGitRepositoryService(suite.store, suite.testDir, "http://localhost:8080", "test", "test@example.com")
}

func (suite *GitRepositoryPushSuiteADO) TestPushBranchToRemote() {
	if suite.adoToken == "" || suite.adoRepo == "" {
		suite.T().Skip("CI_ADO_TOKEN and CI_ADO_REPO environment variables are required")
	}

	ctx := context.Background()
	repoID := "test-repo-push"
	branchName := fmt.Sprintf("ci/%s/test-git-push", time.Now().Format("2006-01-02-15-04-05"))

	bareRepoPath := fmt.Sprintf("%s/%s.git", suite.testDir, repoID)

	cloneOptions := &git.CloneOptions{
		URL:      suite.adoRepo,
		Bare:     true,
		Progress: os.Stdout,
		Auth:     &http.BasicAuth{Username: "PAT", Password: suite.adoToken},
	}

	_, err := git.PlainClone(bareRepoPath, cloneOptions)
	suite.Require().NoError(err, "Failed to clone external repository")

	gitRepo := &types.GitRepository{
		ID:           repoID,
		LocalPath:    bareRepoPath,
		IsExternal:   true,
		ExternalURL:  suite.adoRepo,
		ExternalType: types.ExternalRepositoryTypeADO,
		AzureDevOps: &types.AzureDevOps{
			PersonalAccessToken: suite.adoToken,
		},
		DefaultBranch: "main",
	}

	suite.store.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil).AnyTimes()

	err = suite.s.CreateBranch(ctx, repoID, branchName, "main")
	suite.Require().NoError(err, "Failed to create branch")

	commitMsg := "Test commit - " + time.Now().Format(time.RFC3339)
	_, err = suite.s.CreateOrUpdateFileContents(
		ctx,
		repoID,
		"test-file.txt",
		branchName,
		[]byte("Test content\n"+time.Now().String()),
		commitMsg,
		"Test User",
		"test@example.com",
	)
	suite.Require().NoError(err, "Failed to create commit")

	err = suite.s.PushBranchToRemote(ctx, repoID, branchName, false)
	suite.Require().NoError(err, "Failed to push branch to remote")

	verifyDir := suite.T().TempDir()
	verifyCloneOptions := &git.CloneOptions{
		URL:           suite.adoRepo,
		ReferenceName: plumbing.NewBranchReferenceName(branchName),
		SingleBranch:  true,
		Auth:          &http.BasicAuth{Username: "PAT", Password: suite.adoToken},
	}

	verifyRepo, err := git.PlainClone(verifyDir, verifyCloneOptions)
	suite.Require().NoError(err, "Failed to clone remote to verify push")

	commitIter, err := verifyRepo.Log(&git.LogOptions{})
	suite.Require().NoError(err, "Failed to get commit log from remote")

	var commits []string
	_ = commitIter.ForEach(func(c *object.Commit) error {
		commits = append(commits, c.Message)
		if len(commits) >= 2 {
			return fmt.Errorf("stop")
		}
		return nil
	})

	suite.Require().GreaterOrEqual(len(commits), 1, "Expected at least 1 commit in the remote branch")

	foundCommit := false
	for _, msg := range commits {
		if strings.TrimSpace(msg) == strings.TrimSpace(commitMsg) {
			foundCommit = true
			break
		}
	}

	suite.True(foundCommit, "Commit not found in remote branch, got: %v, expected: %s", commits, commitMsg)
}
