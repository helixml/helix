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

func TestFindFilesResult_ToString(t *testing.T) {
	t.Run("with matches", func(t *testing.T) {
		result := &FindFilesResult{
			Pattern: "*.go",
			Path:    "src",
			Branch:  "main",
			Files:   []string{"src/main.go", "src/utils.go", "src/helper.go"},
			Total:   3,
		}

		output := result.ToString()
		assert.Contains(t, output, "Pattern: *.go")
		assert.Contains(t, output, "Total matches: 3")
		assert.Contains(t, output, "src/main.go")
	})

	t.Run("with no matches", func(t *testing.T) {
		result := &FindFilesResult{
			Pattern: "*.rs",
			Files:   []string{},
			Total:   0,
		}

		output := result.ToString()
		assert.Contains(t, output, "No files found matching pattern")
	})
}

func TestFindFilesTool_Execute(t *testing.T) {
	_, gitRepo, service, mockStore, ctrl := setupTestRepo(t)
	defer ctrl.Finish()

	projectID := "test-project-id"
	project := &types.Project{
		ID:            projectID,
		DefaultRepoID: gitRepo.ID,
	}

	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetGitRepository(gomock.Any(), gitRepo.ID).Return(gitRepo, nil).AnyTimes()

	tool := NewHelixRepositoryFindFilesTool(projectID, mockStore, service)
	ctx := context.Background()
	meta := agent.Meta{}

	t.Run("find go files", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "*.go",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "main.go")
		assert.Contains(t, result, "utils.go")
		assert.Contains(t, result, "main_test.go")
	})

	t.Run("find markdown files", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "*.md",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "README.md")
		assert.Contains(t, result, "architecture.md")
	})

	t.Run("find in specific path", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "*.go",
			"path":    "src",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "src/main.go")
		assert.Contains(t, result, "src/utils.go")
		assert.NotContains(t, result, "tests/")
	})

	t.Run("with limit", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "*.go",
			"limit":   float64(2),
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "Total matches: 2")
	})

	t.Run("no matches", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "*.rs",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "No files found")
	})

	t.Run("missing pattern", func(t *testing.T) {
		args := map[string]interface{}{}

		_, err := tool.Execute(ctx, meta, args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pattern is required")
	})
}
