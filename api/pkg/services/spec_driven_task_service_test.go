package services

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSpecDrivenTaskService_CreateTaskFromPrompt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// Use nil controller since goroutine testing is complex and not critical for this unit test
	mockController := (*controller.Controller)(nil)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, // externalAgentExecutor not needed for tests
		nil, // registerRequestMapping not needed for tests
		nil, // gitRepositoryService not needed for tests
	)
	service.SetTestMode(true)

	ctx := context.Background()
	req := &types.CreateTaskRequest{
		ProjectID: "test-project",
		Prompt:    "Create a user authentication system",
		Type:      "feature",
		Priority:  types.SpecTaskPriorityHigh,
		UserID:    "test-user",
	}

	// Mock expectations
	mockStore.EXPECT().CreateSpecTask(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			assert.Equal(t, "test-project", task.ProjectID)
			assert.Equal(t, "Create a user authentication system", task.OriginalPrompt)
			assert.Equal(t, types.TaskStatusBacklog, task.Status)
			assert.Equal(t, "test-user", task.CreatedBy)
			assert.Equal(t, "feature", task.Type)
			assert.Equal(t, types.SpecTaskPriorityHigh, task.Priority)
			return nil
		},
	)

	// Note: We don't test the goroutine behavior in unit tests due to complexity
	// The spec generation goroutine will fail gracefully with nil controller

	// Execute
	task, err := service.CreateTaskFromPrompt(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, "test-project", task.ProjectID)
	assert.Equal(t, "Create a user authentication system", task.OriginalPrompt)
	assert.Equal(t, types.TaskStatusBacklog, task.Status)
	assert.Equal(t, "test-user", task.CreatedBy)

	// Note: Goroutine will fail gracefully, we only test the synchronous part
}

func TestSpecDrivenTaskService_HandleSpecGenerationComplete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := (*controller.Controller)(nil)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, // externalAgentExecutor not needed for tests
		nil, // registerRequestMapping not needed for tests
		nil, // gitRepositoryService not needed for tests
	)
	service.SetTestMode(true)

	ctx := context.Background()
	taskID := "test-task-id"

	existingTask := &types.SpecTask{
		ID:     taskID,
		Status: types.TaskStatusSpecGeneration,
	}

	specs := &types.SpecGeneration{
		TaskID:             taskID,
		RequirementsSpec:   "Generated requirements specification",
		TechnicalDesign:    "Generated technical design",
		ImplementationPlan: "Generated implementation plan",
		GeneratedAt:        time.Now(),
		ModelUsed:          "test-model",
		TokensUsed:         1500,
	}

	// Mock expectations
	mockStore.EXPECT().GetSpecTask(ctx, taskID).Return(existingTask, nil)
	mockStore.EXPECT().UpdateSpecTask(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			assert.Equal(t, types.TaskStatusSpecReview, task.Status)
			assert.Equal(t, "Generated requirements specification", task.RequirementsSpec)
			assert.Equal(t, "Generated technical design", task.TechnicalDesign)
			assert.Equal(t, "Generated implementation plan", task.ImplementationPlan)
			return nil
		},
	)

	// Execute
	err := service.HandleSpecGenerationComplete(ctx, taskID, specs)

	// Assert
	require.NoError(t, err)
}

func TestSpecDrivenTaskService_ApproveSpecs_Approved(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := (*controller.Controller)(nil)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, // externalAgentExecutor not needed for tests
		nil, // registerRequestMapping not needed for tests
		nil, // gitRepositoryService not needed for tests
	)
	service.SetTestMode(true)

	ctx := context.Background()
	taskID := "test-task-id"

	existingTask := &types.SpecTask{
		ID:                 taskID,
		ProjectID:          "test-project-id",
		Status:             types.TaskStatusSpecReview,
		RequirementsSpec:   "Generated requirements",
		TechnicalDesign:    "Generated design",
		ImplementationPlan: "Generated plan",
	}

	approvalResponse := &types.SpecApprovalResponse{
		TaskID:     taskID,
		Approved:   true,
		ApprovedBy: "test-user",
		ApprovedAt: time.Now(),
	}

	mockProject := &types.Project{
		ID:            "test-project-id",
		DefaultRepoID: "test-repo-id",
	}

	mockRepo := &types.GitRepository{
		ID:            "test-repo-id",
		DefaultBranch: "main",
	}

	// Mock expectations
	mockStore.EXPECT().GetSpecTask(ctx, taskID).Return(existingTask, nil)
	mockStore.EXPECT().GetProject(ctx, "test-project-id").Return(mockProject, nil)
	mockStore.EXPECT().GetGitRepository(ctx, "test-repo-id").Return(mockRepo, nil)
	mockStore.EXPECT().UpdateSpecTask(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			assert.Equal(t, types.TaskStatusImplementation, task.Status)
			assert.Equal(t, "test-user", task.SpecApprovedBy)
			return nil
		},
	)

	// Note: In test mode, the implementation goroutine won't run, so no second update

	// Execute
	err := service.ApproveSpecs(ctx, approvalResponse)

	// Assert
	require.NoError(t, err)

	// Note: In test mode, goroutines don't execute, we only test the synchronous approval
}

func TestSpecDrivenTaskService_ApproveSpecs_Rejected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := (*controller.Controller)(nil)
	var mockPubsub pubsub.PubSub = nil

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
		mockPubsub,
		nil, // externalAgentExecutor not needed for tests
		nil, // registerRequestMapping not needed for tests
		nil, // gitRepositoryService not needed for tests
	)
	service.SetTestMode(true)

	ctx := context.Background()
	taskID := "test-task-id"

	existingTask := &types.SpecTask{
		ID:                 taskID,
		Status:             types.TaskStatusSpecReview,
		RequirementsSpec:   "Generated requirements",
		TechnicalDesign:    "Generated design",
		ImplementationPlan: "Generated plan",
		SpecRevisionCount:  0,
	}

	rejectionResponse := &types.SpecApprovalResponse{
		TaskID:     taskID,
		Approved:   false,
		Comments:   "Needs more detail on authentication flow",
		ApprovedBy: "test-user",
		ApprovedAt: time.Now(),
	}

	// Mock expectations
	mockStore.EXPECT().GetSpecTask(ctx, taskID).Return(existingTask, nil)
	mockStore.EXPECT().UpdateSpecTask(ctx, gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			assert.Equal(t, types.TaskStatusSpecRevision, task.Status)
			assert.Equal(t, 1, task.SpecRevisionCount)
			return nil
		},
	)

	// Execute
	err := service.ApproveSpecs(ctx, rejectionResponse)

	// Assert
	require.NoError(t, err)
}

func TestSpecDrivenTaskService_SelectZedAgent(t *testing.T) {
	// Test with agents available
	service := NewSpecDrivenTaskService(nil, nil, "test-helix-agent", []string{"agent1", "agent2"}, nil, nil, nil, nil)
	agent := service.selectZedAgent()
	assert.Equal(t, "agent1", agent)

	// Test with no agents
	serviceNoAgents := NewSpecDrivenTaskService(nil, nil, "test-helix-agent", []string{}, nil, nil, nil, nil)
	serviceNoAgents.SetTestMode(true)
	agent = serviceNoAgents.selectZedAgent()
	assert.Equal(t, "", agent)
}
