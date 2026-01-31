package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateFileAndBrowseTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	// Create initial file
	require.NoError(t, os.WriteFile(filepath.Join(tempWorkPath, "README.md"), []byte("# Test Repo"), 0644))

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

	repoID := "test-repo-id"
	gitRepo := &types.GitRepository{
		ID:            repoID,
		LocalPath:     bareRepoPath,
		DefaultBranch: "master",
	}

	service := NewGitRepositoryService(
		mockStore,
		testDir,
		"http://localhost:8080",
		"Test User",
		"test@example.com",
	)

	mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil).AnyTimes()

	filePath := "test.txt"
	fileContent := "Hello, World!"
	commitMessage := "Add test file"

	commitHash, err := service.CreateOrUpdateFileContents(
		context.Background(),
		repoID,
		filePath,
		"master",
		[]byte(fileContent),
		commitMessage,
		"Test User",
		"test@example.com",
	)
	require.NoError(t, err)
	assert.NotEmpty(t, commitHash)

	entries, err := service.BrowseTree(context.Background(), repoID, ".", "master")
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	found := false
	for _, entry := range entries {
		if entry.Name == filePath {
			found = true
			assert.False(t, entry.IsDir)
			assert.Equal(t, int64(len(fileContent)), entry.Size)
			break
		}
	}
	assert.True(t, found, "File should be found in tree")

	readContent, err := service.GetFileContents(context.Background(), repoID, filePath, "master")
	require.NoError(t, err)
	assert.Equal(t, fileContent, readContent)
}

func TestBranchIsolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	// Create initial file
	require.NoError(t, os.WriteFile(filepath.Join(tempWorkPath, "README.md"), []byte("# Test Repo"), 0644))

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

	repoID := "test-repo-id"
	gitRepo := &types.GitRepository{
		ID:            repoID,
		LocalPath:     bareRepoPath,
		DefaultBranch: "master",
	}

	service := NewGitRepositoryService(
		mockStore,
		testDir,
		"http://localhost:8080",
		"Test User",
		"test@example.com",
	)

	mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil).AnyTimes()

	err = service.CreateBranch(context.Background(), repoID, "feature-branch", "master")
	require.NoError(t, err)

	filePath := "branch-specific-file.txt"
	fileContent := "This file is only on feature-branch"

	_, err = service.CreateOrUpdateFileContents(
		context.Background(),
		repoID,
		filePath,
		"feature-branch",
		[]byte(fileContent),
		"Add file to feature branch",
		"Test User",
		"test@example.com",
	)
	require.NoError(t, err)

	featureBranchEntries, err := service.BrowseTree(context.Background(), repoID, ".", "feature-branch")
	require.NoError(t, err)

	foundInFeatureBranch := false
	for _, entry := range featureBranchEntries {
		if entry.Name == filePath {
			foundInFeatureBranch = true
			assert.False(t, entry.IsDir)
			break
		}
	}
	assert.True(t, foundInFeatureBranch, "File should be visible on feature-branch")

	masterEntries, err := service.BrowseTree(context.Background(), repoID, ".", "master")
	require.NoError(t, err)

	foundInMaster := false
	for _, entry := range masterEntries {
		if entry.Name == filePath {
			foundInMaster = true
			break
		}
	}
	assert.False(t, foundInMaster, "File should NOT be visible on master branch")
}
