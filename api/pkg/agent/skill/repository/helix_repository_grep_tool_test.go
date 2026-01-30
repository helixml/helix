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

func TestGrepResult_ToString(t *testing.T) {
	t.Run("with matches", func(t *testing.T) {
		result := &GrepResult{
			Pattern: "TODO",
			Path:    "src",
			Branch:  "main",
			Matches: []GrepMatch{
				{File: "src/main.go", LineNumber: 5, Line: "\t// TODO: implement"},
				{File: "src/utils.go", LineNumber: 10, Line: "\t// TODO: add tests"},
			},
			Total: 2,
		}

		output := result.ToString()
		assert.Contains(t, output, "Pattern: TODO")
		assert.Contains(t, output, "Total matches: 2")
		assert.Contains(t, output, "src/main.go:5:")
		assert.Contains(t, output, "TODO: implement")
	})

	t.Run("with no matches", func(t *testing.T) {
		result := &GrepResult{
			Pattern: "FIXME",
			Matches: []GrepMatch{},
			Total:   0,
		}

		output := result.ToString()
		assert.Contains(t, output, "No matches found")
	})
}

func TestGrepTool_Execute(t *testing.T) {
	_, gitRepo, service, mockStore, ctrl := setupTestRepo(t)
	defer ctrl.Finish()

	projectID := "test-project-id"
	project := &types.Project{
		ID:            projectID,
		DefaultRepoID: gitRepo.ID,
	}

	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetGitRepository(gomock.Any(), gitRepo.ID).Return(gitRepo, nil).AnyTimes()

	tool := NewHelixRepositoryGrepTool(projectID, mockStore, service)
	ctx := context.Background()
	meta := agent.Meta{}

	t.Run("search for TODO", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "TODO",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "src/main.go")
		assert.Contains(t, result, "TODO: implement")
	})

	t.Run("search for function definitions", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "func \\w+",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "func main")
		assert.Contains(t, result, "func Helper")
	})

	t.Run("case insensitive search", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern":        "PACKAGE",
			"case_sensitive": false,
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "package main")
	})

	t.Run("search in specific path", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "package",
			"path":    "src",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "src/main.go")
		assert.NotContains(t, result, "tests/")
	})

	t.Run("filter by file pattern", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern":      "package",
			"file_pattern": "*_test.go",
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		assert.Contains(t, result, "tests/main_test.go")
		assert.NotContains(t, result, "src/main.go")
	})

	t.Run("with limit", func(t *testing.T) {
		args := map[string]interface{}{
			"pattern": "package",
			"limit":   float64(2),
		}

		result, err := tool.Execute(ctx, meta, args)
		require.NoError(t, err)
		// Should only return 2 matches
		assert.Contains(t, result, "Total matches: 2")
	})

	t.Run("missing pattern", func(t *testing.T) {
		args := map[string]interface{}{}

		_, err := tool.Execute(ctx, meta, args)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pattern is required")
	})
}
