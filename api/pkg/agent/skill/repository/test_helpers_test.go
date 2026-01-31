package repository

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/setting"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestMain initializes the gitea git module before running tests
func TestMain(m *testing.M) {
	// Set Git home path to temp directory for tests
	setting.Git.HomePath = os.TempDir()

	// Initialize gitea git module
	if err := giteagit.InitSimple(); err != nil {
		panic("failed to initialize git module: " + err.Error())
	}
	os.Exit(m.Run())
}

// setupTestRepo creates a test repository with sample files for testing
func setupTestRepo(t *testing.T) (string, *types.GitRepository, *services.GitRepositoryService, *store.MockStore, *gomock.Controller) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	testDir := t.TempDir()
	bareRepoPath := filepath.Join(testDir, "test-repo.git")
	tempWorkPath := filepath.Join(testDir, "temp-work")
	ctx := context.Background()

	// Initialize bare repository
	err := giteagit.InitRepository(ctx, bareRepoPath, true, "sha1")
	require.NoError(t, err)

	// Initialize working repository
	require.NoError(t, os.MkdirAll(tempWorkPath, 0755))
	err = giteagit.InitRepository(ctx, tempWorkPath, false, "sha1")
	require.NoError(t, err)

	// Create directory structure with sample files
	require.NoError(t, os.MkdirAll(filepath.Join(tempWorkPath, "src"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempWorkPath, "tests"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(tempWorkPath, "docs"), 0755))

	// Create sample files
	files := map[string]string{
		"README.md":            "# Test Repository\n\nThis is a test repository.",
		"src/main.go":          "package main\n\nfunc main() {\n\t// TODO: implement\n}\n",
		"src/utils.go":         "package main\n\nfunc Helper() error {\n\treturn nil\n}\n",
		"tests/main_test.go":   "package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {\n}\n",
		"docs/architecture.md": "# Architecture\n\nSystem architecture documentation.",
		".gitignore":           "*.log\n.env\n",
		"config.yaml":          "app:\n  name: test\n  version: 1.0\n",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempWorkPath, path)
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
	}

	// Add and commit
	err = giteagit.AddChanges(ctx, tempWorkPath, true)
	require.NoError(t, err)

	err = giteagit.CommitChanges(ctx, tempWorkPath, giteagit.CommitChangesOptions{
		Message: "Initial commit",
		Author: &giteagit.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Add bare repo as remote and push
	_, _, err = gitcmd.NewCommand("remote", "add", "origin").
		AddDynamicArguments(bareRepoPath).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: tempWorkPath})
	require.NoError(t, err)

	err = giteagit.Push(ctx, tempWorkPath, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/master:refs/heads/master",
	})
	require.NoError(t, err)

	gitRepo := &types.GitRepository{
		ID:            "test-repo-id",
		LocalPath:     bareRepoPath,
		DefaultBranch: "master",
		IsExternal:    false,
	}

	service := services.NewGitRepositoryService(
		mockStore,
		testDir,
		"http://localhost:8080",
		"Test User",
		"test@example.com",
	)

	return testDir, gitRepo, service, mockStore, ctrl
}
