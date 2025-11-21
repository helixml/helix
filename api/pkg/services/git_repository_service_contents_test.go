package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
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
	repoPath := filepath.Join(testDir, "test-repo")

	bareRepo, err := git.PlainInit(repoPath, true)
	require.NoError(t, err)

	tempClone, err := os.MkdirTemp("", "helix-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	cloneRepo, err := git.PlainInit(tempClone, false)
	require.NoError(t, err)

	worktree, err := cloneRepo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(tempClone, "README.md"), []byte("# Test Repo"), 0644))
	_, err = worktree.Add("README.md")
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	headRef, err := cloneRepo.Head()
	require.NoError(t, err)

	masterRef := plumbing.NewBranchReferenceName("master")
	err = cloneRepo.Storer.SetReference(plumbing.NewHashReference(masterRef, headRef.Hash()))
	require.NoError(t, err)

	_, err = cloneRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoPath},
	})
	require.NoError(t, err)

	err = cloneRepo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/master:refs/heads/master"),
		},
	})
	require.NoError(t, err)

	headRef = plumbing.NewSymbolicReference(plumbing.HEAD, masterRef)
	err = bareRepo.Storer.SetReference(headRef)
	require.NoError(t, err)

	repoID := "test-repo-id"
	gitRepo := &types.GitRepository{
		ID:            repoID,
		LocalPath:     repoPath,
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

	content, err := service.CreateOrUpdateFileContents(
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
	assert.Equal(t, fileContent, content)

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
	repoPath := filepath.Join(testDir, "test-repo")

	bareRepo, err := git.PlainInit(repoPath, true)
	require.NoError(t, err)

	tempClone, err := os.MkdirTemp("", "helix-git-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempClone)

	cloneRepo, err := git.PlainInit(tempClone, false)
	require.NoError(t, err)

	worktree, err := cloneRepo.Worktree()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(tempClone, "README.md"), []byte("# Test Repo"), 0644))
	_, err = worktree.Add("README.md")
	require.NoError(t, err)

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
		},
	})
	require.NoError(t, err)

	headRef, err := cloneRepo.Head()
	require.NoError(t, err)

	masterRef := plumbing.NewBranchReferenceName("master")
	err = cloneRepo.Storer.SetReference(plumbing.NewHashReference(masterRef, headRef.Hash()))
	require.NoError(t, err)

	_, err = cloneRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoPath},
	})
	require.NoError(t, err)

	err = cloneRepo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/master:refs/heads/master"),
		},
	})
	require.NoError(t, err)

	headRef = plumbing.NewSymbolicReference(plumbing.HEAD, masterRef)
	err = bareRepo.Storer.SetReference(headRef)
	require.NoError(t, err)

	repoID := "test-repo-id"
	gitRepo := &types.GitRepository{
		ID:            repoID,
		LocalPath:     repoPath,
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

	initialFiles := []string{"initial-1", "initial-2", "initial-3"}
	for _, fileName := range initialFiles {
		content, err := service.CreateOrUpdateFileContents(
			context.Background(),
			repoID,
			fileName,
			"master",
			[]byte("Initial content for "+fileName),
			"Add initial file "+fileName,
			"Test User",
			"test@example.com",
		)
		require.NoError(t, err)
		assert.Equal(t, "Initial content for "+fileName, content)
	}

	err = service.CreateBranch(context.Background(), repoID, "newly-created-branch", "master")
	require.NoError(t, err)

	newBranchFiles := []string{"new-branch-1", "new-branch-2", "new-branch-3"}
	for _, fileName := range newBranchFiles {
		content, err := service.CreateOrUpdateFileContents(
			context.Background(),
			repoID,
			fileName,
			"newly-created-branch",
			[]byte("New branch content for "+fileName),
			"Add file to new branch "+fileName,
			"Test User",
			"test@example.com",
		)
		require.NoError(t, err)
		assert.Equal(t, "New branch content for "+fileName, content)
	}

	masterEntries, err := service.BrowseTree(context.Background(), repoID, ".", "master")
	require.NoError(t, err)

	masterFileNames := make(map[string]bool)
	for _, entry := range masterEntries {
		if !entry.IsDir {
			masterFileNames[entry.Name] = true
		}
	}

	for _, fileName := range initialFiles {
		assert.True(t, masterFileNames[fileName], "Initial file %s should be visible in master branch", fileName)
	}

	for _, fileName := range newBranchFiles {
		assert.False(t, masterFileNames[fileName], "New branch file %s should NOT be visible in master branch", fileName)
	}

	newBranchEntries, err := service.BrowseTree(context.Background(), repoID, ".", "newly-created-branch")
	require.NoError(t, err)

	newBranchFileNames := make(map[string]bool)
	for _, entry := range newBranchEntries {
		if !entry.IsDir {
			newBranchFileNames[entry.Name] = true
		}
	}

	for _, fileName := range initialFiles {
		assert.True(t, newBranchFileNames[fileName], "Initial file %s should be visible in new branch", fileName)
	}

	for _, fileName := range newBranchFiles {
		assert.True(t, newBranchFileNames[fileName], "New branch file %s should be visible in new branch", fileName)
	}
}
