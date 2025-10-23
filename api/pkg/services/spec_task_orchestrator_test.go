package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockWolfExecutor implements WolfExecutorInterface for testing
type MockWolfExecutor struct {
	mock.Mock
}

func (m *MockWolfExecutor) StartZedAgent(ctx context.Context, agent *types.ZedAgent) (*types.ZedAgentResponse, error) {
	args := m.Called(ctx, agent)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.ZedAgentResponse), args.Error(1)
}

func (m *MockWolfExecutor) StopZedAgent(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

// MockStore implements store.Store interface for testing
type MockStoreOrchestrator struct {
	mock.Mock
}

func (m *MockStoreOrchestrator) GetApp(ctx context.Context, id string) (*types.App, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.App), args.Error(1)
}

func (m *MockStoreOrchestrator) GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.SpecTask), args.Error(1)
}

func (m *MockStoreOrchestrator) UpdateSpecTask(ctx context.Context, task *types.SpecTask) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockStoreOrchestrator) CreateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	args := m.Called(ctx, session)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockStoreOrchestrator) GetSpecTaskExternalAgent(ctx context.Context, specTaskID string) (*types.SpecTaskExternalAgent, error) {
	args := m.Called(ctx, specTaskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.SpecTaskExternalAgent), args.Error(1)
}

func (m *MockStoreOrchestrator) CreateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockStoreOrchestrator) UpdateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockStoreOrchestrator) UpsertExternalAgentActivity(ctx context.Context, activity *types.ExternalAgentActivity) error {
	args := m.Called(ctx, activity)
	return args.Error(0)
}

func TestGetOrCreateExternalAgent_CreateNew(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreOrchestrator)
	mockWolf := new(MockWolfExecutor)

	// Setup orchestrator
	orchestrator := &SpecTaskOrchestrator{
		store:        mockStore,
		wolfExecutor: mockWolf,
		testMode:     true,
	}

	// Test task
	task := &types.SpecTask{
		ID:        "spec_test_new",
		CreatedBy: "user_123",
	}

	// Mock: No existing external agent
	mockStore.On("GetSpecTaskExternalAgent", ctx, "spec_test_new").
		Return(nil, fmt.Errorf("not found"))

	// Mock: Wolf executor creates agent
	mockWolf.On("StartZedAgent", ctx, mock.MatchedBy(func(agent *types.ZedAgent) bool {
		return agent.SessionID == "zed-spectask-spec_test_new" &&
			agent.WorkDir == "/opt/helix/filestore/workspaces/spectasks/spec_test_new"
	})).Return(&types.ZedAgentResponse{
		WolfAppID: "wolf_new_123",
		Status:    "running",
	}, nil)

	// Mock: Create external agent record
	mockStore.On("CreateSpecTaskExternalAgent", ctx, mock.MatchedBy(func(agent *types.SpecTaskExternalAgent) bool {
		return agent.ID == "zed-spectask-spec_test_new" &&
			agent.SpecTaskID == "spec_test_new" &&
			agent.Status == "running"
	})).Return(nil)

	// Mock: Update task with agent ID
	mockStore.On("UpdateSpecTask", ctx, mock.MatchedBy(func(task *types.SpecTask) bool {
		return task.ExternalAgentID == "zed-spectask-spec_test_new"
	})).Return(nil)

	// Execute
	agent, err := orchestrator.getOrCreateExternalAgent(ctx, task)

	// Verify
	require.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, "zed-spectask-spec_test_new", agent.ID)
	assert.Equal(t, "spec_test_new", agent.SpecTaskID)
	assert.Equal(t, "running", agent.Status)

	mockStore.AssertExpectations(t)
	mockWolf.AssertExpectations(t)
}

func TestGetOrCreateExternalAgent_ReuseExisting(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreOrchestrator)
	mockWolf := new(MockWolfExecutor)

	orchestrator := &SpecTaskOrchestrator{
		store:        mockStore,
		wolfExecutor: mockWolf,
		testMode:     true,
	}

	task := &types.SpecTask{
		ID:        "spec_test_existing",
		CreatedBy: "user_456",
	}

	// Mock: Existing agent found and running
	existingAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-spec_test_existing",
		SpecTaskID:      "spec_test_existing",
		WolfAppID:       "wolf_existing_456",
		WorkspaceDir:    "/opt/helix/filestore/workspaces/spectasks/spec_test_existing",
		HelixSessionIDs: []string{"ses_001"},
		Status:          "running",
		Created:         time.Now(),
		LastActivity:    time.Now(),
		UserID:          "user_456",
	}

	mockStore.On("GetSpecTaskExternalAgent", ctx, "spec_test_existing").
		Return(existingAgent, nil)

	// Execute
	agent, err := orchestrator.getOrCreateExternalAgent(ctx, task)

	// Verify - should reuse existing, NOT call Wolf executor
	require.NoError(t, err)
	assert.Equal(t, existingAgent.ID, agent.ID)
	assert.Equal(t, "wolf_existing_456", agent.WolfAppID)

	mockStore.AssertExpectations(t)
	mockWolf.AssertNotCalled(t, "StartZedAgent") // Should NOT create new agent
}

func TestHandleBacklog_PlanningPhase(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreOrchestrator)
	mockWolf := new(MockWolfExecutor)

	orchestrator := &SpecTaskOrchestrator{
		store:        mockStore,
		wolfExecutor: mockWolf,
		testMode:     true,
	}

	// Test task
	task := &types.SpecTask{
		ID:             "spec_planning_test",
		HelixAppID:     "app_zed_agent",
		OriginalPrompt: "Add user authentication",
		CreatedBy:      "user_planning",
	}

	// Mock app
	app := &types.App{
		ID: "app_zed_agent",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:         "Zed Agent",
						Model:        "claude-sonnet-4",
						SystemPrompt: "You are a helpful coding agent",
					},
				},
			},
		},
	}

	// Setup mocks
	mockStore.On("GetApp", ctx, "app_zed_agent").Return(app, nil)
	mockStore.On("GetSpecTaskExternalAgent", ctx, "spec_planning_test").Return(nil, fmt.Errorf("not found"))
	mockWolf.On("StartZedAgent", ctx, mock.Anything).Return(&types.ZedAgentResponse{
		WolfAppID: "wolf_planning_123",
		Status:    "running",
	}, nil)
	mockStore.On("CreateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil)
	mockStore.On("UpdateSpecTask", ctx, mock.Anything).Return(nil)
	mockStore.On("CreateSession", ctx, mock.Anything).Return(&types.Session{
		ID: "ses_planning_spec_planning_test",
	}, nil)
	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil)
	mockStore.On("UpsertExternalAgentActivity", ctx, mock.Anything).Return(nil)

	// Execute
	err := orchestrator.handleBacklog(ctx, task)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, types.TaskStatusSpecGeneration, task.Status)
	assert.Equal(t, "ses_planning_spec_planning_test", task.PlanningSessionID)

	mockStore.AssertExpectations(t)
	mockWolf.AssertExpectations(t)
}

func TestHandleImplementationQueued_ResurrectAgent(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreOrchestrator)
	mockWolf := new(MockWolfExecutor)

	orchestrator := &SpecTaskOrchestrator{
		store:        mockStore,
		wolfExecutor: mockWolf,
		testMode:     true,
	}

	// Task with terminated agent
	task := &types.SpecTask{
		ID:              "spec_impl_test",
		HelixAppID:      "app_zed_agent",
		ExternalAgentID: "zed-spectask-spec_impl_test",
		Name:            "Add Authentication",
		Description:     "Add user authentication feature",
		CreatedBy:       "user_impl",
		RequirementsSpec: "User stories here",
		TechnicalDesign: "Design here",
		ImplementationPlan: "Tasks here",
	}

	app := &types.App{
		ID: "app_zed_agent",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					{
						Name:  "Zed Agent",
						Model: "claude-sonnet-4",
						SystemPrompt: "You are a helpful coding agent",
					},
				},
			},
		},
	}

	// External agent exists but is terminated
	terminatedAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-spec_impl_test",
		SpecTaskID:      "spec_impl_test",
		WolfAppID:       "wolf_old",
		WorkspaceDir:    "/opt/helix/filestore/workspaces/spectasks/spec_impl_test",
		HelixSessionIDs: []string{"ses_planning_spec_impl_test"},
		Status:          "terminated",
		UserID:          "user_impl",
	}

	// Setup mocks
	mockStore.On("GetApp", ctx, "app_zed_agent").Return(app, nil)
	mockStore.On("GetSpecTaskExternalAgent", ctx, "spec_impl_test").Return(terminatedAgent, nil)

	// Should resurrect agent
	mockWolf.On("StartZedAgent", ctx, mock.MatchedBy(func(agent *types.ZedAgent) bool {
		return agent.WorkDir == "/opt/helix/filestore/workspaces/spectasks/spec_impl_test" // SAME workspace!
	})).Return(&types.ZedAgentResponse{
		WolfAppID: "wolf_resurrected_789",
		Status:    "running",
	}, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil).Times(2) // Once for resurrection, once for session
	mockStore.On("CreateSession", ctx, mock.Anything).Return(&types.Session{
		ID: "ses_impl_spec_impl_test",
	}, nil)
	mockStore.On("UpdateSpecTask", ctx, mock.Anything).Return(nil)
	mockStore.On("UpsertExternalAgentActivity", ctx, mock.Anything).Return(nil)

	// Execute
	err := orchestrator.handleImplementationQueued(ctx, task)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, types.TaskStatusImplementation, task.Status)
	assert.Equal(t, "ses_impl_spec_impl_test", task.ImplementationSessionID)

	// Verify wolf executor was called for resurrection
	mockWolf.AssertCalled(t, "StartZedAgent", ctx, mock.Anything)
	mockStore.AssertExpectations(t)
}

func TestHandleImplementationQueued_ReuseRunningAgent(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreOrchestrator)
	mockWolf := new(MockWolfExecutor)

	orchestrator := &SpecTaskOrchestrator{
		store:        mockStore,
		wolfExecutor: mockWolf,
		testMode:     true,
	}

	task := &types.SpecTask{
		ID:              "spec_impl_reuse",
		HelixAppID:      "app_zed_agent",
		ExternalAgentID: "zed-spectask-spec_impl_reuse",
		Name:            "Add Feature",
		Description:     "Add new feature",
		CreatedBy:       "user_reuse",
		RequirementsSpec: "Requirements",
		TechnicalDesign: "Design",
		ImplementationPlan: "Plan",
	}

	app := &types.App{
		ID: "app_zed_agent",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					Name: "Zed Agent",
					Model: "claude-sonnet-4",
					SystemPrompt: "Helpful agent",
				}},
			},
		},
	}

	// External agent is already running
	runningAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-spec_impl_reuse",
		SpecTaskID:      "spec_impl_reuse",
		WolfAppID:       "wolf_running_999",
		WorkspaceDir:    "/opt/helix/filestore/workspaces/spectasks/spec_impl_reuse",
		HelixSessionIDs: []string{"ses_planning_spec_impl_reuse"},
		Status:          "running", // Already running!
		UserID:          "user_reuse",
	}

	// Setup mocks
	mockStore.On("GetApp", ctx, "app_zed_agent").Return(app, nil)
	mockStore.On("GetSpecTaskExternalAgent", ctx, "spec_impl_reuse").Return(runningAgent, nil)
	mockStore.On("CreateSession", ctx, mock.Anything).Return(&types.Session{
		ID: "ses_impl_spec_impl_reuse",
	}, nil)
	mockStore.On("UpdateSpecTask", ctx, mock.Anything).Return(nil)
	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil)
	mockStore.On("UpsertExternalAgentActivity", ctx, mock.Anything).Return(nil)

	// Execute
	err := orchestrator.handleImplementationQueued(ctx, task)

	// Verify
	require.NoError(t, err)

	// Should NOT call Wolf executor (agent already running)
	mockWolf.AssertNotCalled(t, "StartZedAgent")
	mockStore.AssertExpectations(t)
}

func TestBuildPlanningPrompt_MultiRepo(t *testing.T) {
	orchestrator := &SpecTaskOrchestrator{
		testMode: true,
	}

	// Task with multiple repositories
	task := &types.SpecTask{
		ID:             "spec_multi_repo",
		OriginalPrompt: "Add authentication feature with microservices architecture",
		AttachedRepositories: []byte(`[
			{"repository_id": "repo_backend", "clone_url": "http://api:8080/git/repo_backend", "local_path": "backend", "is_primary": true},
			{"repository_id": "repo_frontend", "clone_url": "http://api:8080/git/repo_frontend", "local_path": "frontend", "is_primary": false}
		]`),
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

	// Verify prompt contains:
	assert.Contains(t, prompt, "Add authentication feature") // Original prompt
	assert.Contains(t, prompt, "git clone http://api:8080/git/repo_backend backend") // Backend clone
	assert.Contains(t, prompt, "git clone http://api:8080/git/repo_frontend frontend") // Frontend clone
	assert.Contains(t, prompt, "helix-design-docs") // Worktree setup
	assert.Contains(t, prompt, "requirements.md") // Design doc files
	assert.Contains(t, prompt, "tasks.md") // Task list
	assert.Contains(t, prompt, "task-metadata.json") // Metadata extraction
	assert.Contains(t, prompt, "git push") // Push instructions
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
		RequirementsSpec: "User story: As a user, I want to login...",
		TechnicalDesign: "Architecture: Use JWT tokens...",
		ImplementationPlan: "- [ ] Create user model\n- [ ] Add login endpoint",
		AttachedRepositories: []byte(`[{"repository_id": "repo", "clone_url": "http://api:8080/git/repo", "local_path": "backend", "is_primary": true}]`),
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

	// Verify prompt contains:
	assert.Contains(t, prompt, "User Auth Feature") // Task name
	assert.Contains(t, prompt, "Implement user authentication") // Description
	assert.Contains(t, prompt, "User story: As a user, I want to login") // Requirements
	assert.Contains(t, prompt, "Architecture: Use JWT tokens") // Design
	assert.Contains(t, prompt, "Create user model") // Implementation plan
	assert.Contains(t, prompt, "helix-design-docs") // Worktree reference
	assert.Contains(t, prompt, "feature/spec_impl_prompt") // Feature branch
	assert.Contains(t, prompt, "SAME Zed instance from the planning phase") // Multi-session context
}

func TestSanitizeForBranchName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Add User Authentication", "add-user-authentication"},
		{"Fix: API Bug", "fix-api-bug"},
		{"Refactor Payment_System", "refactor-payment-system"},
		{"Add Dark Mode (UI)", "add-dark-mode-ui"},
		{"Feature #123: New Dashboard", "feature-123-new-dashboard"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeForBranchName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
