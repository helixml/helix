package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestSpecTaskOrchestratorTestSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskOrchestratorTestSuite))
}

type SpecTaskOrchestratorTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	store        *store.MockStore
	executor     *MockContainerExecutor
	gitService   *MockGitService
	orchestrator *SpecTaskOrchestrator
}

func (s *SpecTaskOrchestratorTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = NewMockContainerExecutor(s.ctrl)
	s.gitService = NewMockGitService(s.ctrl)
	s.orchestrator = &SpecTaskOrchestrator{
		store:             s.store,
		containerExecutor: s.executor,
		gitService:        s.gitService,
		testMode:          true,
	}
}

func (s *SpecTaskOrchestratorTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleDone_StopsDesktop() {
	ctx := context.Background()
	task := &types.SpecTask{
		ID:                "task-123",
		PlanningSessionID: "session-456",
		Status:            types.TaskStatusDone,
	}

	s.executor.EXPECT().StopDesktop(ctx, "session-456").Return(nil)

	err := s.orchestrator.handleDone(ctx, task)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleDone_KeepAliveSkipsStop() {
	ctx := context.Background()
	task := &types.SpecTask{
		ID:                "task-keep-alive",
		PlanningSessionID: "session-keep-alive",
		Status:            types.TaskStatusDone,
		KeepAlive:         true,
	}

	// No StopDesktop expectation — gomock will fail the test if it gets called.
	err := s.orchestrator.handleDone(ctx, task)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_SkipsWhenStaleEvent() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	latestTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusQueuedSpecGeneration,
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_RespectsPlanningLimit() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	latestTask := &types.SpecTask{
		ID:           "task-123",
		ProjectID:    "project-123",
		Status:       types.TaskStatusBacklog,
		JustDoItMode: false,
	}

	project := &types.Project{
		ID:                    "project-123",
		AutoStartBacklogTasks: true,
		Metadata: types.ProjectMetadata{
			BoardSettings: &types.BoardSettings{
				WIPLimits: types.WIPLimits{
					Planning: 1,
				},
			},
		},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		{ID: "task-existing", Status: types.TaskStatusSpecGeneration},
		latestTask,
	}, nil)

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

// Review capacity is now gated on the Review column's OWN limit
// (reviewCount >= reviewLimit), not the sum of planning+review. A Review column
// at its limit blocks backlog tasks from entering planning; one planning task in
// flight no longer counts against the review limit.
func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_SkipsWhenReviewColumnFull() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}
	latestTask := &types.SpecTask{
		ID:        eventTask.ID,
		ProjectID: eventTask.ProjectID,
		Status:    types.TaskStatusBacklog,
	}
	project := &types.Project{
		ID:                    eventTask.ProjectID,
		AutoStartBacklogTasks: true,
		Metadata: types.ProjectMetadata{BoardSettings: &types.BoardSettings{
			WIPLimits: types.WIPLimits{Planning: 3, Review: 2},
		}},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	// Review column at its limit (2) → no review capacity → stay in backlog.
	// No UpdateSpecTask expectation: gomock fails if the task is progressed.
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		{ID: "review-1", Status: types.TaskStatusSpecReview},
		{ID: "review-2", Status: types.TaskStatusSpecRevision},
		latestTask,
	}, nil)

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
	s.Equal(types.TaskStatusBacklog, latestTask.Status)
}

// Regression for Problem 2: previously a single planning task + empty Review
// column tripped `planningCount+reviewCount >= reviewLimit` and wrongly blocked
// the backlog task. With the fix (independent limits) it must progress into
// queued_spec_generation.
func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_ProgressesWithPlanningInFlightAndReviewEmpty() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}
	latestTask := &types.SpecTask{
		ID:        eventTask.ID,
		ProjectID: eventTask.ProjectID,
		Status:    types.TaskStatusBacklog,
	}
	project := &types.Project{
		ID:                    eventTask.ProjectID,
		AutoStartBacklogTasks: true,
		Metadata: types.ProjectMetadata{BoardSettings: &types.BoardSettings{
			WIPLimits: types.WIPLimits{Planning: 3, Review: 2},
		}},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		{ID: "planning", Status: types.TaskStatusSpecGeneration},
		latestTask,
	}, nil)
	s.store.EXPECT().UpdateSpecTask(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		s.Equal(types.TaskStatusQueuedSpecGeneration, task.Status)
		return nil
	})

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_RespectsImplementationLimitForJustDoIt() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	latestTask := &types.SpecTask{
		ID:           "task-123",
		ProjectID:    "project-123",
		Status:       types.TaskStatusBacklog,
		JustDoItMode: true,
	}

	project := &types.Project{
		ID:                    "project-123",
		AutoStartBacklogTasks: true,
		Metadata: types.ProjectMetadata{
			BoardSettings: &types.BoardSettings{
				WIPLimits: types.WIPLimits{
					Implementation: 1,
				},
			},
		},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		{ID: "task-existing", Status: types.TaskStatusQueuedImplementation},
		latestTask,
	}, nil)

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_SkipsWhenDependencyNotDone() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	latestTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	project := &types.Project{
		ID:                    "project-123",
		AutoStartBacklogTasks: true,
	}

	latestTaskWithDependencies := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
		DependsOn: []types.SpecTask{
			{ID: "dep-1", Status: types.TaskStatusImplementation},
		},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		latestTaskWithDependencies,
	}, nil)

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_SkipsWhenDependencyNotFullyLoaded() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	latestTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	project := &types.Project{
		ID:                    "project-123",
		AutoStartBacklogTasks: true,
	}

	latestTaskWithDependencies := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
		DependsOn: []types.SpecTask{
			{ID: "dep-1"},
		},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		latestTaskWithDependencies,
	}, nil)

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleBacklog_ProgressesWhenDependencyArchived() {
	ctx := context.Background()
	eventTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	latestTask := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
	}

	project := &types.Project{
		ID:                    "project-123",
		AutoStartBacklogTasks: true,
	}

	latestTaskWithDependencies := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusBacklog,
		DependsOn: []types.SpecTask{
			{ID: "dep-1", Status: types.TaskStatusImplementation, Archived: true},
		},
	}

	s.store.EXPECT().GetSpecTask(ctx, eventTask.ID).Return(latestTask, nil)
	s.store.EXPECT().GetProject(ctx, eventTask.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:     eventTask.ProjectID,
		WithDependsOn: true,
	}).Return([]*types.SpecTask{
		latestTaskWithDependencies,
	}, nil)
	s.store.EXPECT().UpdateSpecTask(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		s.Equal(types.TaskStatusQueuedSpecGeneration, task.Status)
		return nil
	})

	err := s.orchestrator.handleBacklog(ctx, eventTask)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedSpecGeneration_SkipsWhenDependencyNotDone() {
	ctx := context.Background()
	task := &types.SpecTask{
		ID:        "task-123",
		ProjectID: "project-123",
		Status:    types.TaskStatusQueuedSpecGeneration,
		DependsOn: []types.SpecTask{
			{ID: "dep-1", Status: types.TaskStatusImplementation},
		},
	}

	err := s.orchestrator.handleQueuedSpecGeneration(ctx, task)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedSpecGeneration_RespectsReviewCapacity() {
	ctx := context.Background()
	task := &types.SpecTask{
		ID:        "task-queued",
		ProjectID: "project-123",
		Status:    types.TaskStatusQueuedSpecGeneration,
	}
	project := &types.Project{
		ID: task.ProjectID,
		Metadata: types.ProjectMetadata{BoardSettings: &types.BoardSettings{
			WIPLimits: types.WIPLimits{Planning: 3, Review: 2},
		}},
	}

	s.store.EXPECT().GetSpecTask(ctx, task.ID).Return(task, nil)
	s.store.EXPECT().GetProject(ctx, task.ProjectID).Return(project, nil)
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: task.ProjectID}).Return([]*types.SpecTask{
		{ID: "review-1", Status: types.TaskStatusSpecReview},
		{ID: "review-2", Status: types.TaskStatusSpecReview},
		task,
	}, nil)

	err := s.orchestrator.handleQueuedSpecGeneration(ctx, task)
	s.Require().NoError(err)
	s.Equal(types.TaskStatusQueuedSpecGeneration, task.Status)
}

// The planning limit is now effective (Problem 2): with the Planning column at
// its limit, a queued task stays queued regardless of Review. Previously the
// summed formula meant the review reservation always tripped first and this
// limit was dead.
func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedSpecGeneration_RespectsPlanningLimit() {
	ctx := context.Background()
	task := &types.SpecTask{
		ID:        "task-queued",
		ProjectID: "project-123",
		Status:    types.TaskStatusQueuedSpecGeneration,
	}
	project := &types.Project{
		ID: task.ProjectID,
		Metadata: types.ProjectMetadata{BoardSettings: &types.BoardSettings{
			WIPLimits: types.WIPLimits{Planning: 2, Review: 2},
		}},
	}

	s.store.EXPECT().GetSpecTask(ctx, task.ID).Return(task, nil)
	s.store.EXPECT().GetProject(ctx, task.ProjectID).Return(project, nil)
	// 2 tasks generating specs == planning limit, empty Review → stay queued.
	// No UpdateSpecTask expectation: gomock fails if the task is progressed.
	s.store.EXPECT().ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: task.ProjectID}).Return([]*types.SpecTask{
		{ID: "gen-1", Status: types.TaskStatusSpecGeneration},
		{ID: "gen-2", Status: types.TaskStatusSpecGeneration},
		task,
	}, nil)

	err := s.orchestrator.handleQueuedSpecGeneration(ctx, task)
	s.Require().NoError(err)
	s.Equal(types.TaskStatusQueuedSpecGeneration, task.Status)
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleQueuedImplementation_SkipsWhenDependencyNotDone() {
	ctx := context.Background()
	task := &types.SpecTask{
		ID:        "task-456",
		ProjectID: "project-123",
		Status:    types.TaskStatusQueuedImplementation,
		DependsOn: []types.SpecTask{
			{ID: "dep-2", Status: types.TaskStatusSpecGeneration},
		},
	}

	err := s.orchestrator.handleQueuedImplementation(ctx, task)
	s.Require().NoError(err)
}

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

// TestPlanningQueueReason is the table-driven test for the extracted pure gate
// function. It is the single source of truth shared by the orchestrator and the
// read handlers, so it must cover every block cause and the not-blocked case.
func TestPlanningQueueReason(t *testing.T) {
	proj := func(planning, review int) *types.Project {
		return &types.Project{Metadata: types.ProjectMetadata{BoardSettings: &types.BoardSettings{
			WIPLimits: types.WIPLimits{Planning: planning, Review: review},
		}}}
	}
	queued := &types.SpecTask{ID: "t1", ProjectID: "p1", Status: types.TaskStatusQueuedSpecGeneration}

	tests := []struct {
		name         string
		project      *types.Project
		projectTasks []*types.SpecTask
		task         *types.SpecTask
		wantEmpty    bool
		wantContains string
	}{
		{
			name:    "not blocked - capacity available",
			project: proj(3, 2),
			projectTasks: []*types.SpecTask{
				{ID: "a", Status: types.TaskStatusSpecGeneration},
				{ID: "b", Status: types.TaskStatusSpecReview},
				queued,
			},
			task:      queued,
			wantEmpty: true,
		},
		{
			name:    "planning full",
			project: proj(3, 2),
			projectTasks: []*types.SpecTask{
				{ID: "a", Status: types.TaskStatusSpecGeneration},
				{ID: "b", Status: types.TaskStatusSpecGeneration},
				{ID: "c", Status: types.TaskStatusSpecGeneration},
				queued,
			},
			task:         queued,
			wantContains: "planning capacity",
		},
		{
			name:    "review full",
			project: proj(3, 2),
			projectTasks: []*types.SpecTask{
				{ID: "r1", Status: types.TaskStatusSpecReview},
				{ID: "r2", Status: types.TaskStatusSpecRevision},
				queued,
			},
			task:         queued,
			wantContains: "review capacity",
		},
		{
			name:    "dependency blocked names the blocking task",
			project: proj(3, 2),
			projectTasks: []*types.SpecTask{
				{ID: "dep-1", Name: "Login flow", Status: types.TaskStatusImplementation},
			},
			task: &types.SpecTask{ID: "t2", ProjectID: "p1", Status: types.TaskStatusQueuedSpecGeneration,
				DependsOn: []types.SpecTask{{ID: "dep-1", Status: types.TaskStatusImplementation}}},
			wantContains: "Login flow",
		},
		{
			// Problem 2 regression: 2 planning (< limit 3) + empty Review must NOT
			// block. The old summed formula (planningCount+reviewCount >= reviewLimit)
			// blocked here because 2 >= 2.
			name:    "planning limit no longer dead",
			project: proj(3, 2),
			projectTasks: []*types.SpecTask{
				{ID: "a", Status: types.TaskStatusSpecGeneration},
				{ID: "b", Status: types.TaskStatusSpecGeneration},
				queued,
			},
			task:      queued,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanningQueueReason(tt.project, tt.projectTasks, tt.task)
			if tt.wantEmpty {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.wantContains)
			}
		})
	}
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
		{"Task with\nnewline", "task-with-newline"},                         // Newlines become hyphens
		{"Task with\ttab", "task-with-tab"},                                 // Tabs become hyphens
		{"Connect to Azure DevOps\n^ in dialog", "connect-to-azure-devops"}, // Regression test
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeForBranchName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func (s *SpecTaskOrchestratorTestSuite) TestHandleSpecApproved_SelfHealsNilSpecApproval() {
	ctx := context.Background()
	approvedAt := time.Now()

	// Simulate the broken state: spec_approved status but SpecApproval is nil
	task := &types.SpecTask{
		ID:             "task-stuck",
		ProjectID:      "project-1",
		Status:         types.TaskStatusSpecApproved,
		SpecApprovedBy: "user-1",
		SpecApprovedAt: &approvedAt,
		SpecApproval:   nil,
		TaskNumber:     42,
		Name:           "stuck-task",
	}

	// Set up the specTaskService on the orchestrator
	service := NewSpecDrivenTaskService(
		s.store, nil, "test-helix-agent", []string{"test-zed-agent"},
		nil, nil, nil, nil, NewDisabledKoditService(),
	)
	service.SetTestMode(true)
	s.orchestrator.specTaskService = service

	// ApproveSpecs re-reads the task from store
	s.store.EXPECT().GetSpecTask(ctx, "task-stuck").Return(task, nil)
	s.store.EXPECT().GetProject(ctx, "project-1").Return(&types.Project{
		ID:            "project-1",
		DefaultRepoID: "repo-1",
	}, nil)
	s.store.EXPECT().GetGitRepository(ctx, "repo-1").Return(&types.GitRepository{
		ID:            "repo-1",
		DefaultBranch: "main",
	}, nil)
	s.store.EXPECT().TransitionSpecTaskStatus(
		ctx,
		"task-stuck",
		gomock.Any(),
		types.TaskStatusImplementation,
		gomock.Any(),
	).Return(true, nil)

	err := s.orchestrator.handleSpecApproved(ctx, task)
	s.Require().NoError(err)
}

func (s *SpecTaskOrchestratorTestSuite) TestIsDeletedProjectError() {
	// GORM "record not found" errors should match
	assert.True(s.T(), isDeletedProjectError(fmt.Errorf("failed to get project: record not found")))
	assert.True(s.T(), isDeletedProjectError(fmt.Errorf("record not found")))

	// Domain errors containing "not found" but NOT "record not found" should NOT match
	assert.False(s.T(), isDeletedProjectError(fmt.Errorf("spec approval not found")))
	assert.False(s.T(), isDeletedProjectError(fmt.Errorf("failed to approve specs: spec approval not found")))
	assert.False(s.T(), isDeletedProjectError(fmt.Errorf("default repository not set for project")))
	assert.False(s.T(), isDeletedProjectError(fmt.Errorf("branch not found")))
}

func (s *SpecTaskOrchestratorTestSuite) TestProcessTask_ErrorFilterDistinguishesNotFoundTypes() {
	ctx := context.Background()

	// Verify processTask dispatches to handleSpecApproved
	task := &types.SpecTask{
		ID:         "task-test",
		ProjectID:  "project-1",
		Status:     types.TaskStatusSpecApproved,
		TaskNumber: 99,
		Name:       "test-task",
	}

	service := NewSpecDrivenTaskService(
		s.store, nil, "test-helix-agent", []string{},
		nil, nil, nil, nil, NewDisabledKoditService(),
	)
	service.SetTestMode(true)
	s.orchestrator.specTaskService = service

	// GetSpecTask returns a task where ApproveSpecs will hit GetProject and fail
	s.store.EXPECT().GetSpecTask(ctx, "task-test").Return(task, nil)
	s.store.EXPECT().GetProject(ctx, "project-1").Return(&types.Project{
		ID:            "project-1",
		DefaultRepoID: "",
	}, nil)

	err := s.orchestrator.processTask(ctx, task)
	s.Require().Error(err)
	// The error should contain "not found" in a domain-specific way, not "record not found"
	assert.Contains(s.T(), err.Error(), "default repository not set")
	assert.False(s.T(), isDeletedProjectError(err))
}

// makePullRequestTask returns a task in pull_request status with the given
// number of tracked PRs and no branch (so the branch-merge fallback path
// inside processExternalPullRequestStatus early-exits without needing
// store/repo mocks).
func makePullRequestTask(prCount int) *types.SpecTask {
	prs := make([]types.RepoPR, prCount)
	for i := 0; i < prCount; i++ {
		prs[i] = types.RepoPR{
			RepositoryID: fmt.Sprintf("repo-%d", i+1),
			PRID:         fmt.Sprintf("%d", i+1),
			PRNumber:     i + 1,
			PRState:      "open",
		}
	}
	return &types.SpecTask{
		ID:               "task-pr-1",
		Status:           types.TaskStatusPullRequest,
		BranchName:       "", // skips IsBranchMerged fallback
		RepoPullRequests: prs,
	}
}

// Regression test for the pod-restart-triggered "wrongly merged" bug.
// Before the fix, if GetPullRequest returned an error for every tracked PR
// in a single poll cycle, the `allMerged` flag stayed at its `true` default
// and the task was incorrectly transitioned to TaskStatusDone with
// MergedToMain=true. Customer-visible as tasks moving from the Pull Request
// kanban column to Merged after Helm upgrades / GitLab transient errors.
func (s *SpecTaskOrchestratorTestSuite) TestProcessExternalPullRequestStatus_AllErrors_StaysInPullRequest() {
	ctx := context.Background()
	task := makePullRequestTask(2)

	gitErr := fmt.Errorf("simulated gitlab 502")
	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-1", "1").
		Return(nil, gitErr)
	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-2", "2").
		Return(nil, gitErr)

	// No UpdateSpecTask expectation — gomock will fail the test if the
	// function tries to persist a transition.
	err := s.orchestrator.processExternalPullRequestStatus(ctx, task)
	s.Require().NoError(err)

	assert.Equal(s.T(), types.TaskStatusPullRequest, task.Status,
		"task must stay in pull_request when every GetPullRequest errors")
	assert.False(s.T(), task.MergedToMain, "MergedToMain must not be set on error")
	assert.Nil(s.T(), task.MergedAt, "MergedAt must not be set on error")
}

// Sanity: when every PR genuinely is merged, the task DOES transition to
// done. Guards against an over-eager fix that breaks the happy path.
func (s *SpecTaskOrchestratorTestSuite) TestProcessExternalPullRequestStatus_AllMerged_TransitionsToDone() {
	ctx := context.Background()
	task := makePullRequestTask(2)

	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-1", "1").
		Return(&types.PullRequest{State: types.PullRequestStateMerged}, nil)
	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-2", "2").
		Return(&types.PullRequest{State: types.PullRequestStateMerged}, nil)
	s.store.EXPECT().UpdateSpecTask(ctx, gomock.Any()).Return(nil)
	s.store.EXPECT().DismissAttentionEventsForTask(ctx, "task-pr-1").Return(int64(0), nil)

	err := s.orchestrator.processExternalPullRequestStatus(ctx, task)
	s.Require().NoError(err)

	assert.Equal(s.T(), types.TaskStatusDone, task.Status)
	assert.True(s.T(), task.MergedToMain)
	assert.NotNil(s.T(), task.MergedAt)
}

// Production-realistic case: task has a BranchName set, all PRs error, so
// the IsBranchMerged fallback runs. With active PR branches (commits not
// in default), IsBranchMerged returns false and the task correctly stays
// in pull_request. Important regression test because my primary fix only
// addresses the allMerged-true path; the fallback path is a separate
// transition site that could in principle wrongly transition on stale
// repo state. This test pins the safe production-typical scenario.
func (s *SpecTaskOrchestratorTestSuite) TestProcessExternalPullRequestStatus_AllErrorsWithBranch_FallbackDoesNotTransition() {
	ctx := context.Background()
	task := makePullRequestTask(1)
	task.BranchName = "feature/active-work"
	task.ProjectID = "proj-1"

	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-1", "1").
		Return(nil, fmt.Errorf("simulated gitlab 502"))
	s.store.EXPECT().
		GetProject(ctx, "proj-1").
		Return(&types.Project{ID: "proj-1", DefaultRepoID: "repo-default"}, nil)
	s.store.EXPECT().
		GetGitRepository(ctx, "repo-default").
		Return(&types.GitRepository{ID: "repo-default", DefaultBranch: "main"}, nil)
	// Active PR branch: HEAD has commits not in default → not an ancestor.
	s.gitService.EXPECT().
		IsBranchMerged(ctx, "repo-default", "feature/active-work", "main").
		Return(false, nil)

	// No UpdateSpecTask — fallback found nothing, no state changed.
	err := s.orchestrator.processExternalPullRequestStatus(ctx, task)
	s.Require().NoError(err)

	assert.Equal(s.T(), types.TaskStatusPullRequest, task.Status)
	assert.False(s.T(), task.MergedToMain)
}

// Sanity: when one PR is merged but the other errors, we cannot conclude
// "all merged" — task must stay in pull_request until we can re-confirm.
// This is the case where the fix genuinely differs from the pre-fix bug.
func (s *SpecTaskOrchestratorTestSuite) TestProcessExternalPullRequestStatus_MergedPlusError_StaysInPullRequest() {
	ctx := context.Background()
	task := makePullRequestTask(2)

	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-1", "1").
		Return(&types.PullRequest{State: types.PullRequestStateMerged}, nil)
	s.gitService.EXPECT().
		GetPullRequest(ctx, "repo-2", "2").
		Return(nil, fmt.Errorf("simulated gitlab 502"))

	// Persisted because one PR's state flipped from "open" to "merged"
	// in task.RepoPullRequests. But task.Status must stay pull_request.
	s.store.EXPECT().UpdateSpecTask(ctx, gomock.Any()).Return(nil)

	err := s.orchestrator.processExternalPullRequestStatus(ctx, task)
	s.Require().NoError(err)

	assert.Equal(s.T(), types.TaskStatusPullRequest, task.Status,
		"task must stay in pull_request when at least one PR errors")
	assert.False(s.T(), task.MergedToMain)
}
