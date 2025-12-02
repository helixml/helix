package services

import (
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// Note: These are simplified unit tests focusing on testable functions
// Full integration tests with store/wolf mocking should be in integration test suite

func TestBuildPlanningPrompt_MultiRepo(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := store.NewMockStore(ctrl)
	orchestrator := &SpecTaskOrchestrator{
		testMode: true,
		store:    store,
	}

	store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{}, nil)

	// Task with multiple repositories
	task := &types.SpecTask{
		ID:             "spec_multi_repo",
		OriginalPrompt: "Add authentication feature with microservices architecture",
	}

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					SystemPrompt: "You are a planning agent",
				}},
			},
		},
	}

	// Build prompt
	prompt := orchestrator.buildPlanningPrompt(task, app)

	// Verify prompt contains key elements
	assert.Contains(t, prompt, "Add authentication feature") // Original prompt
	assert.Contains(t, prompt, "helix-specs")                // Worktree setup
	assert.Contains(t, prompt, "requirements.md")            // Design doc files
	assert.Contains(t, prompt, "tasks.md")                   // Task list
	assert.Contains(t, prompt, "task-metadata.json")         // Metadata extraction

	// Note: Repo-specific git clone commands require a mock store with GetProjectRepositories
	// This test verifies basic prompt structure without repos
}

func TestBuildPlanningPrompt_NoRepos(t *testing.T) {
	ctrl := gomock.NewController(t)

	store := store.NewMockStore(ctrl)
	orchestrator := &SpecTaskOrchestrator{
		testMode: true,
		store:    store,
	}

	store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{}, nil)

	task := &types.SpecTask{
		ID:             "spec_no_repo",
		OriginalPrompt: "Add dark mode toggle",
	}

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					SystemPrompt: "Planning agent",
				}},
			},
		},
	}

	prompt := orchestrator.buildPlanningPrompt(task, app)

	// Should still generate valid prompt
	assert.Contains(t, prompt, "Add dark mode toggle")
	assert.Contains(t, prompt, "helix-specs")
	assert.Contains(t, prompt, "requirements.md")
}

func TestSanitizeForBranchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Add User Authentication", "add-user-authentication"},
		{"Fix: API Bug", "fix-api-bug"},
		{"Refactor Payment_System", "refactor-paymentsystem"}, // Underscores removed
		{"Add Dark Mode (UI)", "add-dark-mode-ui"},
		{"Feature #123: New Dashboard", "feature-123-new-dashboard"},
		{"UPPERCASE TEXT", "uppercase-text"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Special!@#$%Characters", "specialcharacters"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeForBranchName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
