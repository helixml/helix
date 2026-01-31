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

func TestListCommits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	testDir := t.TempDir()
	repoPath := filepath.Join(testDir, "test-repo")
	ctx := context.Background()

	// Initialize repository
	err := giteagit.InitRepository(ctx, repoPath, false, "sha1")
	require.NoError(t, err)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	commits := []struct {
		message string
		file    string
		content string
		time    time.Time
	}{
		{"Initial commit", "README.md", "# Test Project", baseTime},
		{"Add feature A", "feature_a.go", "package main", baseTime.Add(1 * time.Hour)},
		{"Add feature B", "feature_b.go", "package main", baseTime.Add(2 * time.Hour)},
		{"Fix bug", "bugfix.go", "package main", baseTime.Add(3 * time.Hour)},
		{"Update docs", "docs.md", "# Docs", baseTime.Add(4 * time.Hour)},
	}

	for _, c := range commits {
		filePath := filepath.Join(repoPath, c.file)
		require.NoError(t, os.WriteFile(filePath, []byte(c.content), 0644))

		err = giteagit.AddChanges(ctx, repoPath, true)
		require.NoError(t, err)

		// Use gitcmd with custom env to set commit date
		// gitea's CommitChanges ignores the When field, so we use git commit directly
		dateStr := c.time.Format(time.RFC3339)
		_, _, gitErr := gitcmd.NewCommand().
			AddConfig("user.name", "Test Author").
			AddConfig("user.email", "test@example.com").
			AddArguments("commit").
			AddOptionFormat("--message=%s", c.message).
			AddOptionFormat("--date=%s", dateStr).
			RunStdString(ctx, &gitcmd.RunOpts{
				Dir: repoPath,
				Env: []string{
					"GIT_AUTHOR_DATE=" + dateStr,
					"GIT_COMMITTER_DATE=" + dateStr,
				},
			})
		require.NoError(t, gitErr)
	}

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

	t.Run("list all commits", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 5)
		assert.Equal(t, "Update docs", resp.Commits[0].Message)
		assert.Equal(t, "Initial commit", resp.Commits[4].Message)
	})

	t.Run("list commits with pagination", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID:  repoID,
			PerPage: 2,
			Page:    1,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 2)
		assert.Equal(t, "Update docs", resp.Commits[0].Message)
		assert.Equal(t, "Fix bug", resp.Commits[1].Message)
	})

	t.Run("list commits page 2", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID:  repoID,
			PerPage: 2,
			Page:    2,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 2)
		assert.Equal(t, "Add feature B", resp.Commits[0].Message)
		assert.Equal(t, "Add feature A", resp.Commits[1].Message)
	})

	t.Run("list commits with since filter", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		since := baseTime.Add(2 * time.Hour).Add(30 * time.Minute).Format(time.RFC3339)
		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Since:  since,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 2)
		assert.Equal(t, "Update docs", resp.Commits[0].Message)
		assert.Equal(t, "Fix bug", resp.Commits[1].Message)
	})

	t.Run("list commits with until filter", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		until := baseTime.Add(2 * time.Hour).Add(30 * time.Minute).Format(time.RFC3339)
		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Until:  until,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 3)
		assert.Equal(t, "Add feature B", resp.Commits[0].Message)
		assert.Equal(t, "Add feature A", resp.Commits[1].Message)
		assert.Equal(t, "Initial commit", resp.Commits[2].Message)
	})

	t.Run("list commits with since and until filter", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		since := baseTime.Add(1 * time.Hour).Add(30 * time.Minute).Format(time.RFC3339)
		until := baseTime.Add(3 * time.Hour).Add(30 * time.Minute).Format(time.RFC3339)
		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Since:  since,
			Until:  until,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 2)
		assert.Equal(t, "Fix bug", resp.Commits[0].Message)
		assert.Equal(t, "Add feature B", resp.Commits[1].Message)
	})

	t.Run("list commits with branch", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		// Create a new branch using git command
		_, _, gitErr := gitcmd.NewCommand("branch", "test-branch").
			RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
		require.NoError(t, gitErr)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Branch: "test-branch",
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 5)
	})

	t.Run("verify commit fields", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		require.NotEmpty(t, resp.Commits)

		commit := resp.Commits[0]
		assert.NotEmpty(t, commit.SHA)
		assert.Equal(t, "Update docs", commit.Message)
		assert.Equal(t, "Test Author", commit.Author)
		assert.Equal(t, "test@example.com", commit.Email)
		assert.False(t, commit.Timestamp.IsZero())
	})

	t.Run("default pagination values", func(t *testing.T) {
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID:  repoID,
			PerPage: 0,
			Page:    0,
		}

		resp, err := service.ListCommits(context.Background(), req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Commits, 5)
	})
}

func TestListCommits_ErrorCases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	testDir := t.TempDir()
	ctx := context.Background()

	service := NewGitRepositoryService(
		mockStore,
		testDir,
		"http://localhost:8080",
		"Test User",
		"test@example.com",
	)

	t.Run("repository not found", func(t *testing.T) {
		repoID := "non-existent-repo"
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(nil, assert.AnError)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
		}

		resp, err := service.ListCommits(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("repository without local path", func(t *testing.T) {
		// When a non-external repo has no LocalPath, GetRepository returns "repository not found"
		// because it tries to verify the default path exists
		repoID := "repo-no-path"
		gitRepo := &types.GitRepository{
			ID:        repoID,
			LocalPath: "",
		}

		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
		}

		resp, err := service.ListCommits(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "repository not found")
	})

	t.Run("invalid since date format", func(t *testing.T) {
		repoPath := filepath.Join(testDir, "test-repo")

		// Initialize repository
		err := giteagit.InitRepository(ctx, repoPath, false, "sha1")
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Test"), 0644))
		err = giteagit.AddChanges(ctx, repoPath, true)
		require.NoError(t, err)

		err = giteagit.CommitChanges(ctx, repoPath, giteagit.CommitChangesOptions{
			Message: "Initial",
			Author: &giteagit.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Now(),
			},
			Committer: &giteagit.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

		repoID := "test-repo"
		gitRepo := &types.GitRepository{
			ID:            repoID,
			LocalPath:     repoPath,
			DefaultBranch: "master",
		}

		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Since:  "invalid-date",
		}

		resp, err := service.ListCommits(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid since date format")
	})

	t.Run("invalid until date format", func(t *testing.T) {
		repoPath := filepath.Join(testDir, "test-repo-2")

		// Initialize repository
		err := giteagit.InitRepository(ctx, repoPath, false, "sha1")
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Test"), 0644))
		err = giteagit.AddChanges(ctx, repoPath, true)
		require.NoError(t, err)

		err = giteagit.CommitChanges(ctx, repoPath, giteagit.CommitChangesOptions{
			Message: "Initial",
			Author: &giteagit.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Now(),
			},
			Committer: &giteagit.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

		repoID := "test-repo-2"
		gitRepo := &types.GitRepository{
			ID:            repoID,
			LocalPath:     repoPath,
			DefaultBranch: "master",
		}

		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Until:  "invalid-date",
		}

		resp, err := service.ListCommits(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "invalid until date format")
	})

	t.Run("invalid branch", func(t *testing.T) {
		repoPath := filepath.Join(testDir, "test-repo-3")

		// Initialize repository
		err := giteagit.InitRepository(ctx, repoPath, false, "sha1")
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Test"), 0644))
		err = giteagit.AddChanges(ctx, repoPath, true)
		require.NoError(t, err)

		err = giteagit.CommitChanges(ctx, repoPath, giteagit.CommitChangesOptions{
			Message: "Initial",
			Author: &giteagit.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Now(),
			},
			Committer: &giteagit.Signature{
				Name:  "Test",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)

		repoID := "test-repo-3"
		gitRepo := &types.GitRepository{
			ID:            repoID,
			LocalPath:     repoPath,
			DefaultBranch: "master",
		}

		mockStore.EXPECT().GetGitRepository(gomock.Any(), repoID).Return(gitRepo, nil)

		req := &types.ListCommitsRequest{
			RepoID: repoID,
			Branch: "non-existent-branch",
		}

		resp, err := service.ListCommits(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "failed to get branch commit")
	})
}
