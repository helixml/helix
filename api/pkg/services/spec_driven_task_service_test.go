package services

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockStore implements the Store interface for testing
type MockStore struct {
	mock.Mock
}

func (m *MockStore) CreateSpecTask(ctx context.Context, task *types.SpecTask) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockStore) GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(*types.SpecTask), args.Error(1)
}

func (m *MockStore) UpdateSpecTask(ctx context.Context, task *types.SpecTask) error {
	args := m.Called(ctx, task)
	return args.Error(0)
}

func (m *MockStore) ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	args := m.Called(ctx, filters)
	return args.Get(0).([]*types.SpecTask), args.Error(1)
}

// Add other Store interface methods as no-ops
func (m *MockStore) CreateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	return nil, nil
}
func (m *MockStore) GetOrganization(ctx context.Context, q *types.GetOrganizationQuery) (*types.Organization, error) {
	return nil, nil
}
func (m *MockStore) UpdateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	return nil, nil
}
func (m *MockStore) DeleteOrganization(ctx context.Context, id string) error { return nil }
func (m *MockStore) ListOrganizations(ctx context.Context, query *types.ListOrganizationsQuery) ([]*types.Organization, error) {
	return nil, nil
}

// ... (truncated for brevity - in real implementation, add all Store interface methods)

// MockController implements the Controller interface for testing
type MockController struct {
	mock.Mock
}

func (m *MockController) CreateSession(ctx context.Context, req *types.CreateSessionRequest) (*types.Session, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockController) CreateMessage(ctx context.Context, req *types.CreateMessageRequest) (*types.Message, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*types.Message), args.Error(1)
}

func TestSpecDrivenTaskService_CreateTaskFromPrompt(t *testing.T) {
	// Setup
	mockStore := &MockStore{}
	mockController := &MockController{}

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
	)

	ctx := context.Background()
	req := &CreateTaskRequest{
		ProjectID: "test-project",
		Prompt:    "Create a user authentication system",
		Type:      "feature",
		Priority:  "high",
		UserID:    "test-user",
	}

	// Mock expectations
	mockStore.On("CreateSpecTask", ctx, mock.MatchedBy(func(task *types.SpecTask) bool {
		return task.ProjectID == "test-project" &&
			task.OriginalPrompt == "Create a user authentication system" &&
			task.Status == types.TaskStatusBacklog &&
			task.CreatedBy == "test-user"
	})).Return(nil)

	mockStore.On("UpdateSpecTask", ctx, mock.MatchedBy(func(task *types.SpecTask) bool {
		return task.Status == types.TaskStatusSpecGeneration
	})).Return(nil)

	mockController.On("CreateSession", ctx, mock.MatchedBy(func(req *types.CreateSessionRequest) bool {
		return req.UserID == "test-user" &&
			req.ProjectID == "test-project" &&
			req.SessionMode == types.SessionModeInference
	})).Return(&types.Session{
		ID: "test-session-id",
	}, nil)

	mockController.On("CreateMessage", ctx, mock.MatchedBy(func(req *types.CreateMessageRequest) bool {
		return req.Content == "Create a user authentication system"
	})).Return(&types.Message{
		ID: "test-message-id",
	}, nil)

	// Execute
	task, err := service.CreateTaskFromPrompt(ctx, req)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, task)
	assert.Equal(t, "test-project", task.ProjectID)
	assert.Equal(t, "Create a user authentication system", task.OriginalPrompt)
	assert.Equal(t, types.TaskStatusBacklog, task.Status)
	assert.Equal(t, "test-user", task.CreatedBy)

	// Give the goroutine a moment to execute
	time.Sleep(100 * time.Millisecond)

	mockStore.AssertExpectations(t)
	mockController.AssertExpectations(t)
}

func TestSpecDrivenTaskService_ApproveSpecs(t *testing.T) {
	// Setup
	mockStore := &MockStore{}
	mockController := &MockController{}

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
	)

	ctx := context.Background()
	taskID := "test-task-id"

	existingTask := &types.SpecTask{
		ID:                 taskID,
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

	// Mock expectations
	mockStore.On("GetSpecTask", ctx, taskID).Return(existingTask, nil)
	mockStore.On("UpdateSpecTask", ctx, mock.MatchedBy(func(task *types.SpecTask) bool {
		return task.Status == types.TaskStatusSpecApproved &&
			task.SpecApprovedBy == "test-user"
	})).Return(nil).Twice() // Called twice: once for approval, once for implementation start

	// Execute
	err := service.ApproveSpecs(ctx, approvalResponse)

	// Assert
	assert.NoError(t, err)

	// Give the goroutine a moment to execute
	time.Sleep(100 * time.Millisecond)

	mockStore.AssertExpectations(t)
}

func TestSpecDrivenTaskService_ApproveSpecs_Rejection(t *testing.T) {
	// Setup
	mockStore := &MockStore{}
	mockController := &MockController{}

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
	)

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
	mockStore.On("GetSpecTask", ctx, taskID).Return(existingTask, nil)
	mockStore.On("UpdateSpecTask", ctx, mock.MatchedBy(func(task *types.SpecTask) bool {
		return task.Status == types.TaskStatusSpecRevision &&
			task.SpecRevisionCount == 1
	})).Return(nil)

	// Execute
	err := service.ApproveSpecs(ctx, rejectionResponse)

	// Assert
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestSpecDrivenTaskService_HandleSpecGenerationComplete(t *testing.T) {
	// Setup
	mockStore := &MockStore{}
	mockController := &MockController{}

	service := NewSpecDrivenTaskService(
		mockStore,
		mockController,
		"test-helix-agent",
		[]string{"test-zed-agent"},
	)

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
	mockStore.On("GetSpecTask", ctx, taskID).Return(existingTask, nil)
	mockStore.On("UpdateSpecTask", ctx, mock.MatchedBy(func(task *types.SpecTask) bool {
		return task.Status == types.TaskStatusSpecReview &&
			task.RequirementsSpec == "Generated requirements specification" &&
			task.TechnicalDesign == "Generated technical design" &&
			task.ImplementationPlan == "Generated implementation plan"
	})).Return(nil)

	// Execute
	err := service.HandleSpecGenerationComplete(ctx, taskID, specs)

	// Assert
	assert.NoError(t, err)
	mockStore.AssertExpectations(t)
}

func TestSpecDrivenTaskService_BuildSpecGenerationPrompt(t *testing.T) {
	// Setup
	service := NewSpecDrivenTaskService(nil, nil, "test-helix-agent", []string{"test-zed-agent"})

	task := &types.SpecTask{
		ProjectID: "test-project",
		Type:      "feature",
		Priority:  "high",
	}

	// Execute
	prompt := service.buildSpecGenerationPrompt(task)

	// Assert
	assert.Contains(t, prompt, "software specification expert")
	assert.Contains(t, prompt, "test-project")
	assert.Contains(t, prompt, "feature")
	assert.Contains(t, prompt, "high")
	assert.Contains(t, prompt, "Requirements Specification")
	assert.Contains(t, prompt, "Technical Design")
	assert.Contains(t, prompt, "Implementation Plan")
}

func TestSpecDrivenTaskService_BuildImplementationPrompt(t *testing.T) {
	// Setup
	service := NewSpecDrivenTaskService(nil, nil, "test-helix-agent", []string{"test-zed-agent"})

	task := &types.SpecTask{
		Name:               "User Authentication System",
		OriginalPrompt:     "Create a user authentication system",
		RequirementsSpec:   "Generated requirements",
		TechnicalDesign:    "Generated design",
		ImplementationPlan: "Generated plan",
	}

	// Execute
	prompt := service.buildImplementationPrompt(task)

	// Assert
	assert.Contains(t, prompt, "senior software engineer")
	assert.Contains(t, prompt, "User Authentication System")
	assert.Contains(t, prompt, "Create a user authentication system")
	assert.Contains(t, prompt, "Generated requirements")
	assert.Contains(t, prompt, "Generated design")
	assert.Contains(t, prompt, "Generated plan")
	assert.Contains(t, prompt, "APPROVED SPECIFICATIONS")
}
