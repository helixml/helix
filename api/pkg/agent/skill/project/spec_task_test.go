package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
