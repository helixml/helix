package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestListFilesResult_ToString(t *testing.T) {
	t.Run("with files and directories", func(t *testing.T) {
		result := &ListFilesResult{
			Path:   "src",
			Branch: "main",
			Entries: []types.TreeEntry{
				{Name: "utils", Path: "src/utils", IsDir: true, Size: 0},
				{Name: "main.go", Path: "src/main.go", IsDir: false, Size: 1024},
				{Name: "helper.go", Path: "src/helper.go", IsDir: false, Size: 512},
			},
			Total: 3,
		}

		output := result.ToString()
		assert.Contains(t, output, "Path: src")
		assert.Contains(t, output, "Branch: main")
		assert.Contains(t, output, "Total entries: 3")
		assert.Contains(t, output, "[DIR]  utils/")
		assert.Contains(t, output, "[FILE] main.go")
		assert.Contains(t, output, "1.00 KB")
	})

	t.Run("with empty directory", func(t *testing.T) {
		result := &ListFilesResult{
			Path:    "empty",
			Branch:  "main",
			Entries: []types.TreeEntry{},
			Total:   0,
		}

		output := result.ToString()
		assert.Contains(t, output, "(empty directory)")
	})
}

func TestListFilesTool_Execute(t *testing.T) {
	_, gitRepo, service, mockStore, ctrl := setupTestRepo(t)
	defer ctrl.Finish()

	projectID := "test-project-id"
	project := &types.Project{
		ID:            projectID,
		DefaultRepoID: gitRepo.ID,
	}

	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetGitRepository(gomock.Any(), gitRepo.ID).Return(gitRepo, nil).AnyTimes()

	tool := NewHelixRepositoryListFilesTool(projectID, mockStore, service)
	ctx := context.Background()
	meta := agent.Meta{}

	t.Run("list root directory", func(t *testing.T) {
		args := map[string]interface{}{
			"path": ".",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "README.md")
		assert.Contains(t, result, "src/")
		assert.Contains(t, result, "tests/")
		assert.Contains(t, result, "docs/")
	})

	t.Run("list subdirectory", func(t *testing.T) {
		args := map[string]interface{}{
			"path": "src",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "main.go")
		assert.Contains(t, result, "utils.go")
	})

	t.Run("with limit", func(t *testing.T) {
		args := map[string]interface{}{
			"path":  ".",
			"limit": float64(2),
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "Total entries: 2")
	})

	t.Run("missing path defaults to root", func(t *testing.T) {
		args := map[string]interface{}{}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "README.md")
	})

	t.Run("project not found", func(t *testing.T) {
		mockStore.EXPECT().GetProject(gomock.Any(), "invalid-project").Return(nil, fmt.Errorf("not found"))

		tool := NewHelixRepositoryListFilesTool("invalid-project", mockStore, service)
		args := map[string]interface{}{}

		_, err := tool.Execute(ctx, meta, args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get project")
	})
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1572864, "1.50 MB"},
		{1073741824, "1.00 GB"},
		{1610612736, "1.50 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatSize(tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}
