package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSpecTaskSummary_ToString(t *testing.T) {
	t.Run("with all fields populated including times", func(t *testing.T) {
		startedAt := time.Date(2025, 1, 3, 10, 30, 0, 0, time.UTC)
		completedAt := time.Date(2025, 1, 3, 12, 45, 0, 0, time.UTC)

		summary := &SpecTaskSummary{
			ID:             "task-123",
			Name:           "Implement login feature",
			Description:    "Add OAuth2 authentication",
			Status:         "in_progress",
			Priority:       "high",
			BranchName:     "feature/login",
			PullRequestID:  "pr-456",
			PullRequestURL: "https://github.com/org/repo/pull/456",
			StartedAt:      &startedAt,
			CompletedAt:    &completedAt,
		}

		result := summary.ToString()

		assert.Contains(t, result, "ID: task-123")
		assert.Contains(t, result, "Task: Implement login feature")
		assert.Contains(t, result, "Description: Add OAuth2 authentication")
		assert.Contains(t, result, "Status: in_progress")
		assert.Contains(t, result, "Priority: high")
		assert.Contains(t, result, "BranchName: feature/login")
		assert.Contains(t, result, "PullRequestID: pr-456")
		assert.Contains(t, result, "PullRequestURL: https://github.com/org/repo/pull/456")
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
		assert.NotContains(t, result, "PullRequestID:")
		assert.NotContains(t, result, "PullRequestURL:")
	})
}

func TestListSpecTasksResult_ToString(t *testing.T) {
	t.Run("with multiple tasks", func(t *testing.T) {
		startedAt := time.Date(2025, 1, 3, 9, 0, 0, 0, time.UTC)

		result := &ListSpecTasksResult{
			Tasks: []SpecTaskSummary{
				{
					ID:          "task-1",
					Name:        "First task",
					Description: "Description 1",
					Status:      "done",
					Priority:    "high",
				},
				{
					ID:          "task-2",
					Name:        "Second task",
					Description: "Description 2",
					Status:      "in_progress",
					Priority:    "medium",
					StartedAt:   &startedAt,
				},
			},
			Total: 2,
		}

		output := result.ToString()

		assert.Contains(t, output, "Total Tasks: 2")
		assert.Contains(t, output, "--- Task 1 ---")
		assert.Contains(t, output, "ID: task-1")
		assert.Contains(t, output, "Task: First task")
		assert.Contains(t, output, "--- Task 2 ---")
		assert.Contains(t, output, "ID: task-2")
		assert.Contains(t, output, "Task: Second task")
		assert.Contains(t, output, "StartedAt: 2025-01-03T09:00:00Z")
	})

	t.Run("with empty task list", func(t *testing.T) {
		result := &ListSpecTasksResult{
			Tasks: []SpecTaskSummary{},
			Total: 0,
		}

		output := result.ToString()

		assert.Contains(t, output, "Total Tasks: 0")
		assert.NotContains(t, output, "--- Task")
	})

	t.Run("with single task", func(t *testing.T) {
		result := &ListSpecTasksResult{
			Tasks: []SpecTaskSummary{
				{
					ID:          "only-task",
					Name:        "Only task",
					Description: "Single task description",
					Status:      "backlog",
					Priority:    "critical",
				},
			},
			Total: 1,
		}

		output := result.ToString()

		assert.Contains(t, output, "Total Tasks: 1")
		assert.Contains(t, output, "--- Task 1 ---")
		assert.Contains(t, output, "ID: only-task")
		assert.NotContains(t, output, "--- Task 2 ---")
	})
}
