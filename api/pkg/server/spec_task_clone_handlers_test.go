package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCloneTaskToProject_WithSpecs_SetsDesignDocsPushedAt(t *testing.T) {
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

	// Verify the cloned task has DesignDocsPushedAt set
	require.NotNil(t, createdTask, "CreateSpecTask should have been called")
	assert.NotNil(t, createdTask.DesignDocsPushedAt, "DesignDocsPushedAt should be set when source has specs")

	// Verify the status is spec_review (skipping spec generation)
	assert.Equal(t, types.TaskStatusSpecReview, createdTask.Status, "Status should be spec_review when source has specs")

	// Verify the DesignDocsPushedAt is recent (within last minute)
	assert.WithinDuration(t, time.Now(), *createdTask.DesignDocsPushedAt, time.Minute)

	// Verify specs were copied
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
