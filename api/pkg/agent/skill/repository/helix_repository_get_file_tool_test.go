package repository

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetFileResult_ToString(t *testing.T) {
	result := &GetFileResult{
		Path:    "src/main.go",
		Branch:  "main",
		Content: "package main\n\nfunc main() {}\n",
		Size:    28,
	}

	output := result.ToString()
	assert.Contains(t, output, "File: src/main.go")
	assert.Contains(t, output, "Branch: main")
	assert.Contains(t, output, "Size: 28 B")
	assert.Contains(t, output, "package main")
}

func TestGetFileTool_Execute(t *testing.T) {
	_, gitRepo, service, mockStore, ctrl := setupTestRepo(t)
	defer ctrl.Finish()

	projectID := "test-project-id"
	project := &types.Project{
		ID:            projectID,
		DefaultRepoID: gitRepo.ID,
	}

	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetGitRepository(gomock.Any(), gitRepo.ID).Return(gitRepo, nil).AnyTimes()

	tool := NewHelixRepositoryGetFileTool(projectID, mockStore, service)
	ctx := context.Background()
	meta := agent.Meta{}

	t.Run("get go file", func(t *testing.T) {
		args := map[string]interface{}{
			"path": "src/main.go",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "package main")
		assert.Contains(t, result, "func main()")
		assert.Contains(t, result, "TODO: implement")
	})

	t.Run("get markdown file", func(t *testing.T) {
		args := map[string]interface{}{
			"path": "README.md",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "# Test Repository")
		assert.Contains(t, result, "This is a test repository")
	})

	t.Run("get config file", func(t *testing.T) {
		args := map[string]interface{}{
			"path": "config.yaml",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "app:")
		assert.Contains(t, result, "name: test")
	})

	t.Run("file not found", func(t *testing.T) {
		args := map[string]interface{}{
			"path": "nonexistent.txt",
		}

		_, err := tool.Execute(ctx, meta, args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})

	t.Run("missing path", func(t *testing.T) {
		args := map[string]interface{}{}

		_, err := tool.Execute(ctx, meta, args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "path is required")
	})
}
