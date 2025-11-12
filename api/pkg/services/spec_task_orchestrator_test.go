package services

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

// Note: These are simplified unit tests focusing on testable functions
// Full integration tests with store/wolf mocking should be in integration test suite

func TestBuildPlanningPrompt_MultiRepo(t *testing.T) {
	orchestrator := &SpecTaskOrchestrator{
		testMode: true,
	}

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
	assert.Contains(t, prompt, "git clone http://api:8080/git/repo_backend backend") // Backend clone
	assert.Contains(t, prompt, "git clone http://api:8080/git/repo_frontend frontend") // Frontend clone
	assert.Contains(t, prompt, "helix-specs") // Worktree setup
	assert.Contains(t, prompt, "requirements.md") // Design doc files
	assert.Contains(t, prompt, "tasks.md") // Task list
	assert.Contains(t, prompt, "task-metadata.json") // Metadata extraction
}

func TestBuildPlanningPrompt_NoRepos(t *testing.T) {
	orchestrator := &SpecTaskOrchestrator{
		testMode: true,
	}

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

func TestBuildImplementationPrompt_IncludesSpecs(t *testing.T) {
	orchestrator := &SpecTaskOrchestrator{
		testMode: true,
	}

	task := &types.SpecTask{
		ID:             "spec_impl_prompt",
		Name:           "User Auth Feature",
		Description:    "Implement user authentication",
		OriginalPrompt: "Add user auth",
		RequirementsSpec: "User story: As a user, I want to login securely",
		TechnicalDesign: "Architecture: Use JWT tokens with refresh mechanism",
		ImplementationPlan: "- [ ] Create user model\n- [ ] Add login endpoint\n- [ ] Implement JWT generation",
	}

	app := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					SystemPrompt: "You are an implementation agent",
				}},
			},
		},
	}

	// Build prompt
	prompt := orchestrator.buildImplementationPrompt(task, app)

	// Verify prompt contains all context
	assert.Contains(t, prompt, "User Auth Feature") // Task name
	assert.Contains(t, prompt, "Implement user authentication") // Description
	assert.Contains(t, prompt, "User story: As a user, I want to login") // Requirements
	assert.Contains(t, prompt, "Architecture: Use JWT tokens") // Design
	assert.Contains(t, prompt, "Create user model") // Implementation plan
	assert.Contains(t, prompt, "helix-specs") // Worktree reference
	assert.Contains(t, prompt, "feature/spec_impl_prompt") // Feature branch
	assert.Contains(t, prompt, "SAME Zed instance from the planning phase") // Multi-session context
	assert.Contains(t, prompt, "[ ] -> [~]") // Task markers
	assert.Contains(t, prompt, "[~] -> [x]") // Completion markers
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
