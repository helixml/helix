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

	initialCommitTime := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)

	// Create initial files
	require.NoError(t, os.WriteFile(filepath.Join(tempWorkPath, "README.md"), []byte("# Test Repo"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempWorkPath, "docs"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(tempWorkPath, "docs", "guide.md"), []byte("main"), 0644))

	// Add and commit
	err = giteagit.AddChanges(ctx, tempWorkPath, true)
	require.NoError(t, err)

	_, _, err = gitcmd.NewCommand().
		AddOptionValues("-c", "user.name=Test Author").
		AddOptionValues("-c", "user.email=test@example.com").
		AddArguments("commit").
		AddOptionFormat("--date=%s", initialCommitTime.Format(time.RFC3339)).
		AddOptionFormat("--message=%s", "Initial commit").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: tempWorkPath})
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

	filePath := "docs/guide.md"
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

	var featureDirectoryCommit time.Time
	for _, entry := range featureBranchEntries {
		if entry.Name == "docs" {
			require.NotNil(t, entry.LastCommitAt)
			featureDirectoryCommit = *entry.LastCommitAt
			assert.True(t, entry.IsDir)
			break
		}
	}
	require.False(t, featureDirectoryCommit.IsZero())
	assert.True(t, featureDirectoryCommit.After(initialCommitTime))

	featureDirectoryEntries, err := service.BrowseTree(context.Background(), repoID, "docs", "feature-branch")
	require.NoError(t, err)
	require.Len(t, featureDirectoryEntries, 1)
	require.NotNil(t, featureDirectoryEntries[0].LastCommitAt)
	assert.Equal(t, featureDirectoryCommit, *featureDirectoryEntries[0].LastCommitAt)

	masterEntries, err := service.BrowseTree(context.Background(), repoID, ".", "master")
	require.NoError(t, err)

	var masterDirectoryCommit time.Time
	for _, entry := range masterEntries {
		if entry.Name == "docs" {
			require.NotNil(t, entry.LastCommitAt)
			masterDirectoryCommit = *entry.LastCommitAt
			break
		}
	}
	assert.True(t, initialCommitTime.Equal(masterDirectoryCommit))

	masterDirectoryEntries, err := service.BrowseTree(context.Background(), repoID, "docs", "master")
	require.NoError(t, err)
	require.Len(t, masterDirectoryEntries, 1)
	require.NotNil(t, masterDirectoryEntries[0].LastCommitAt)
	assert.True(t, initialCommitTime.Equal(*masterDirectoryEntries[0].LastCommitAt))
}
