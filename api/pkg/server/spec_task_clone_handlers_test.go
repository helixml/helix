package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCloneTaskToProject_WithSpecs_GoesToSpecGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	sourceTask := &types.SpecTask{
		ID:                 "source-task-id",
		ProjectID:          "source-project-id",
		Name:               "Test Task",
		RequirementsSpec:   "# Requirements\nSome requirements",
		TechnicalDesign:    "# Technical Design\nSome design",
		ImplementationPlan: "# Implementation Plan\nSome plan",
		JustDoItMode:       false,
	}

	targetProjectID := "target-project-id"
	cloneGroupID := "clone-group-id"
	userID := "user-id"
	userEmail := "user@example.com"

	// Mock GetProject
	mockStore.EXPECT().GetProject(gomock.Any(), targetProjectID).Return(&types.Project{
		ID:   targetProjectID,
		Name: "Target Project",
	}, nil)

	// Mock GetLatestDesignReview - no design review exists
	mockStore.EXPECT().GetLatestDesignReview(gomock.Any(), sourceTask.ID).Return(nil, store.ErrNotFound)

	// Mock IncrementGlobalTaskNumber
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(42, nil)

	// Mock CreateSpecTask - capture the created task to verify it
	var createdTask *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			createdTask = task
			return nil
		},
	)

	// Call cloneTaskToProject with autoStart=true
	result, err := server.cloneTaskToProject(context.Background(), sourceTask, targetProjectID, cloneGroupID, userID, userEmail, true)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the cloned task goes to queued_spec_generation to boot the desktop.
	// The agent will see pre-populated specs and adapt them, then push to set DesignDocsPushedAt.
	require.NotNil(t, createdTask, "CreateSpecTask should have been called")
	assert.Equal(t, types.TaskStatusQueuedSpecGeneration, createdTask.Status, "Status should be queued_spec_generation to boot desktop")

	// DesignDocsPushedAt is NOT set at clone time - it will be set when agent pushes specs
	assert.Nil(t, createdTask.DesignDocsPushedAt, "DesignDocsPushedAt should not be set at clone time")

	// Verify specs were copied (will be pre-populated in helix-specs/ by StartSpecGeneration)
	assert.Equal(t, sourceTask.RequirementsSpec, createdTask.RequirementsSpec)
	assert.Equal(t, sourceTask.TechnicalDesign, createdTask.TechnicalDesign)
	assert.Equal(t, sourceTask.ImplementationPlan, createdTask.ImplementationPlan)
}

func TestCloneTaskToProject_WithoutSpecs_DoesNotSetDesignDocsPushedAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	// Source task without specs
	sourceTask := &types.SpecTask{
		ID:           "source-task-id",
		ProjectID:    "source-project-id",
		Name:         "Test Task",
		JustDoItMode: false,
	}

	targetProjectID := "target-project-id"
	cloneGroupID := "clone-group-id"
	userID := "user-id"
	userEmail := "user@example.com"

	// Mock GetProject
	mockStore.EXPECT().GetProject(gomock.Any(), targetProjectID).Return(&types.Project{
		ID:   targetProjectID,
		Name: "Target Project",
	}, nil)

	// Mock GetLatestDesignReview - no design review exists
	mockStore.EXPECT().GetLatestDesignReview(gomock.Any(), sourceTask.ID).Return(nil, store.ErrNotFound)

	// Mock IncrementGlobalTaskNumber
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(42, nil)

	// Mock CreateSpecTask - capture the created task to verify it
	var createdTask *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			createdTask = task
			return nil
		},
	)

	// Call cloneTaskToProject with autoStart=true
	result, err := server.cloneTaskToProject(context.Background(), sourceTask, targetProjectID, cloneGroupID, userID, userEmail, true)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the cloned task does NOT have DesignDocsPushedAt set
	require.NotNil(t, createdTask, "CreateSpecTask should have been called")
	assert.Nil(t, createdTask.DesignDocsPushedAt, "DesignDocsPushedAt should NOT be set when source has no specs")

	// Verify the status is queued_spec_generation (needs spec generation)
	assert.Equal(t, types.TaskStatusQueuedSpecGeneration, createdTask.Status, "Status should be queued_spec_generation when source has no specs")
}

func TestCloneTaskToProject_JustDoItMode_SkipsSpecReview(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	// Source task with JustDoItMode enabled (even with specs)
	sourceTask := &types.SpecTask{
		ID:                 "source-task-id",
		ProjectID:          "source-project-id",
		Name:               "Test Task",
		RequirementsSpec:   "# Requirements\nSome requirements",
		TechnicalDesign:    "# Technical Design\nSome design",
		ImplementationPlan: "# Implementation Plan\nSome plan",
		JustDoItMode:       true,
	}

	targetProjectID := "target-project-id"
	cloneGroupID := "clone-group-id"
	userID := "user-id"
	userEmail := "user@example.com"

	// Mock GetProject
	mockStore.EXPECT().GetProject(gomock.Any(), targetProjectID).Return(&types.Project{
		ID:   targetProjectID,
		Name: "Target Project",
	}, nil)

	// Mock GetLatestDesignReview - no design review exists
	mockStore.EXPECT().GetLatestDesignReview(gomock.Any(), sourceTask.ID).Return(nil, store.ErrNotFound)

	// Mock IncrementGlobalTaskNumber
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(42, nil)

	// Mock CreateSpecTask - capture the created task to verify it
	var createdTask *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			createdTask = task
			return nil
		},
	)

	// Call cloneTaskToProject with autoStart=true
	result, err := server.cloneTaskToProject(context.Background(), sourceTask, targetProjectID, cloneGroupID, userID, userEmail, true)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the cloned task goes straight to implementation (JustDoItMode)
	require.NotNil(t, createdTask, "CreateSpecTask should have been called")
	assert.Equal(t, types.TaskStatusQueuedImplementation, createdTask.Status, "JustDoItMode tasks should go to queued_implementation")

	// DesignDocsPushedAt should not be set for JustDoItMode
	assert.Nil(t, createdTask.DesignDocsPushedAt, "DesignDocsPushedAt should not be set for JustDoItMode")
}

// TestCloneTaskToProject_SpecsFromDesignReview_GoesToSpecGeneration tests the scenario
// where the source task has no specs on its direct fields, but has a design review with specs.
// This is a common case when a task was implemented and its specs were pushed to helix-specs,
// but the task record's RequirementsSpec/TechnicalDesign/ImplementationPlan fields are empty.
func TestCloneTaskToProject_SpecsFromDesignReview_GoesToSpecGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	// Source task with NO specs on direct fields (common after implementation)
	sourceTask := &types.SpecTask{
		ID:           "source-task-id",
		ProjectID:    "source-project-id",
		Name:         "Test Task",
		JustDoItMode: false,
		// RequirementsSpec, TechnicalDesign, ImplementationPlan are intentionally empty
	}

	targetProjectID := "target-project-id"
	cloneGroupID := "clone-group-id"
	userID := "user-id"
	userEmail := "user@example.com"

	// Mock GetProject
	mockStore.EXPECT().GetProject(gomock.Any(), targetProjectID).Return(&types.Project{
		ID:   targetProjectID,
		Name: "Target Project",
	}, nil)

	// Mock GetLatestDesignReview - returns a design review WITH specs
	// This simulates the case where specs were pushed to helix-specs during implementation
	mockStore.EXPECT().GetLatestDesignReview(gomock.Any(), sourceTask.ID).Return(&types.SpecTaskDesignReview{
		ID:                 "review-id",
		SpecTaskID:         sourceTask.ID,
		RequirementsSpec:   "# Requirements\nSpecs from design review",
		TechnicalDesign:    "# Technical Design\nSpecs from design review",
		ImplementationPlan: "# Implementation Plan\nSpecs from design review",
	}, nil)

	// Mock IncrementGlobalTaskNumber
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(42, nil)

	// Mock CreateSpecTask - capture the created task to verify it
	var createdTask *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			createdTask = task
			return nil
		},
	)

	// Call cloneTaskToProject with autoStart=true
	result, err := server.cloneTaskToProject(context.Background(), sourceTask, targetProjectID, cloneGroupID, userID, userEmail, true)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the cloned task goes to queued_spec_generation to boot the desktop.
	// The agent will see pre-populated specs and adapt them to the new project.
	require.NotNil(t, createdTask, "CreateSpecTask should have been called")
	assert.Equal(t, types.TaskStatusQueuedSpecGeneration, createdTask.Status, "Status should be queued_spec_generation to boot desktop")

	// DesignDocsPushedAt is NOT set at clone time - it will be set when agent pushes specs
	assert.Nil(t, createdTask.DesignDocsPushedAt, "DesignDocsPushedAt should not be set at clone time")

	// Verify specs were copied from the design review (will be pre-populated in helix-specs/)
	assert.Equal(t, "# Requirements\nSpecs from design review", createdTask.RequirementsSpec)
	assert.Equal(t, "# Technical Design\nSpecs from design review", createdTask.TechnicalDesign)
	assert.Equal(t, "# Implementation Plan\nSpecs from design review", createdTask.ImplementationPlan)
}

func TestCloneTaskToProject_AutoStartFalse_GoesToBacklog(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	sourceTask := &types.SpecTask{
		ID:                 "source-task-id",
		ProjectID:          "source-project-id",
		Name:               "Test Task",
		RequirementsSpec:   "# Requirements\nSome requirements",
		TechnicalDesign:    "# Technical Design\nSome design",
		ImplementationPlan: "# Implementation Plan\nSome plan",
		JustDoItMode:       false,
	}

	targetProjectID := "target-project-id"
	cloneGroupID := "clone-group-id"
	userID := "user-id"
	userEmail := "user@example.com"

	// Mock GetProject
	mockStore.EXPECT().GetProject(gomock.Any(), targetProjectID).Return(&types.Project{
		ID:   targetProjectID,
		Name: "Target Project",
	}, nil)

	// Mock GetLatestDesignReview - no design review exists
	mockStore.EXPECT().GetLatestDesignReview(gomock.Any(), sourceTask.ID).Return(nil, store.ErrNotFound)

	// Mock IncrementGlobalTaskNumber
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(42, nil)

	// Mock CreateSpecTask - capture the created task to verify it
	var createdTask *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, task *types.SpecTask) error {
			createdTask = task
			return nil
		},
	)

	// Call cloneTaskToProject with autoStart=false
	result, err := server.cloneTaskToProject(context.Background(), sourceTask, targetProjectID, cloneGroupID, userID, userEmail, false)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify the cloned task goes to backlog when autoStart is false
	require.NotNil(t, createdTask, "CreateSpecTask should have been called")
	assert.Equal(t, types.SpecTaskStatus("backlog"), createdTask.Status, "Status should be backlog when autoStart is false")

	// DesignDocsPushedAt should not be set when autoStart is false
	assert.Nil(t, createdTask.DesignDocsPushedAt, "DesignDocsPushedAt should not be set when autoStart is false")
}
