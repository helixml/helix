package project

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProjectSpecTaskToolsHidePreparingTasks(t *testing.T) {
	for _, test := range []struct {
		name string
		tool func(store.Store) agent.Tool
		args map[string]interface{}
	}{
		{name: "get", tool: func(st store.Store) agent.Tool { return NewGetSpecTaskTool("project-1", st) }, args: map[string]interface{}{"task_id": "task-1"}},
		{name: "update", tool: func(st store.Store) agent.Tool { return NewUpdateSpecTaskTool("project-1", st) }, args: map[string]interface{}{"task_id": "task-1", "name": "changed"}},
		{name: "start", tool: func(st store.Store) agent.Tool { return NewStartSpecTaskTool("project-1", st) }, args: map[string]interface{}{"task_id": "task-1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().GetSpecTask(gomock.Any(), "task-1").Return(&types.SpecTask{
				ID: "task-1", ProjectID: "project-1", Status: types.TaskStatusPreparing,
			}, nil)

			_, err := test.tool(mockStore).Execute(context.Background(), agent.Meta{}, test.args)
			require.EqualError(t, err, "spec task not found")
		})
	}
}

func TestUpdateSpecTaskToolRejectsPreparingTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "task-1").Return(&types.SpecTask{
		ID: "task-1", ProjectID: "project-1", Status: types.TaskStatusBacklog,
	}, nil)

	_, err := NewUpdateSpecTaskTool("project-1", mockStore).Execute(
		context.Background(),
		agent.Meta{},
		map[string]interface{}{"task_id": "task-1", "status": string(types.TaskStatusPreparing)},
	)
	require.EqualError(t, err, "preparing is an internal task status")
}

func TestSpecTaskSummary_ToString(t *testing.T) {
	t.Run("with all fields", func(t *testing.T) {
		startedAt := time.Date(2025, 1, 3, 10, 30, 0, 0, time.UTC)
		completedAt := time.Date(2025, 1, 3, 12, 45, 0, 0, time.UTC)

		summary := &SpecTaskSummary{
			ID:          "task-123",
			Name:        "Implement login feature",
			Description: "Add OAuth2 authentication",
			Status:      "in_progress",
			Priority:    "high",
			BranchName:  "feature/login",
			RepoPullRequests: []RepoPRSummary{
				{RepositoryName: "main-repo", PRURL: "https://github.com/org/repo/pull/456"},
			},
			StartedAt:   &startedAt,
			CompletedAt: &completedAt,
		}

		result := summary.ToString()

		assert.Contains(t, result, "ID: task-123")
		assert.Contains(t, result, "Task: Implement login feature")
		assert.Contains(t, result, "Description: Add OAuth2 authentication")
		assert.Contains(t, result, "Status: in_progress")
		assert.Contains(t, result, "Priority: high")
		assert.Contains(t, result, "BranchName: feature/login")
		assert.Contains(t, result, "main-repo")
		assert.Contains(t, result, "https://github.com/org/repo/pull/456")
		assert.Contains(t, result, "StartedAt: 2025-01-03T10:30:00Z")
		assert.Contains(t, result, "CompletedAt: 2025-01-03T12:45:00Z")
	})

	t.Run("with nil times omits time fields", func(t *testing.T) {
		summary := &SpecTaskSummary{
			ID:          "task-789",
			Name:        "Bug fix",
			Description: "Fix null pointer",
			Status:      "backlog",
			Priority:    "medium",
			StartedAt:   nil,
			CompletedAt: nil,
		}

		result := summary.ToString()

		assert.NotContains(t, result, "StartedAt:")
		assert.NotContains(t, result, "CompletedAt:")
	})

	t.Run("with empty optional fields omits them", func(t *testing.T) {
		summary := &SpecTaskSummary{
			ID:          "task-empty",
			Name:        "Simple task",
			Description: "",
			Status:      "done",
			Priority:    "low",
		}

		result := summary.ToString()

		assert.Contains(t, result, "ID: task-empty")
		assert.NotContains(t, result, "Description:")
		assert.NotContains(t, result, "BranchName:")
		assert.NotContains(t, result, "PullRequests:")
	})
}

func TestListSpecTasksResult_ToString(t *testing.T) {
	t.Run("with multiple tasks", func(t *testing.T) {
		result := &ListSpecTasksResult{
			Tasks: []SpecTaskSummary{
				{ID: "t1", Name: "Task 1", Status: "done"},
				{ID: "t2", Name: "Task 2", Status: "in_progress"},
			},
			Total: 2,
		}

		output := result.ToString()

		assert.Contains(t, output, "Task 1")
		assert.Contains(t, output, "Task 2")
		assert.Contains(t, output, "Total Tasks: 2")
	})

	t.Run("with no tasks", func(t *testing.T) {
		result := &ListSpecTasksResult{
			Tasks: []SpecTaskSummary{},
			Total: 0,
		}

		output := result.ToString()

		assert.Contains(t, output, "Total Tasks: 0")
	})
}
