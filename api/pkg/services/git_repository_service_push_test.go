package services

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
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

	// Clone using gitea (with authentication embedded in URL)
	authURL := suite.buildAuthenticatedURL(suite.adoRepo, suite.adoToken)
	err := giteagit.Clone(ctx, authURL, bareRepoPath, giteagit.CloneRepoOptions{
		Bare:   true,
		Mirror: true,
	})
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

	// Verify push by cloning to a new directory
	verifyDir := suite.T().TempDir()
	verifyAuthURL := suite.buildAuthenticatedURL(suite.adoRepo, suite.adoToken)

	err = giteagit.Clone(ctx, verifyAuthURL, verifyDir, giteagit.CloneRepoOptions{
		Branch: branchName,
	})
	suite.Require().NoError(err, "Failed to clone remote to verify push")

	// Get recent commits using git log
	stdout, _, err := gitcmd.NewCommand("log", "--oneline", "-2").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: verifyDir})
	suite.Require().NoError(err, "Failed to get commit log from remote")

	commits := strings.Split(strings.TrimSpace(stdout), "\n")
	suite.Require().GreaterOrEqual(len(commits), 1, "Expected at least 1 commit in the remote branch")

	// Verify commit message is present
	foundCommit := false
	for _, line := range commits {
		if strings.Contains(line, "Test commit") {
			foundCommit = true
			break
		}
	}

	suite.True(foundCommit, "Commit not found in remote branch, got: %v, expected message containing: %s", commits, "Test commit")
}

// buildAuthenticatedURL embeds credentials into the URL for Azure DevOps
func (suite *GitRepositoryPushSuiteADO) buildAuthenticatedURL(repoURL, token string) string {
	// For ADO, use PAT as password with any username
	// URL format: https://oauth2:TOKEN@dev.azure.com/...
	if token == "" {
		return repoURL
	}

	// Parse and reconstruct with embedded credentials
	if strings.HasPrefix(repoURL, "https://") {
		return strings.Replace(repoURL, "https://", "https://oauth2:"+token+"@", 1)
	}
	return repoURL
}
