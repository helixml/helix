package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSpecTaskMultiSessionManager_CreateImplementationSessions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	// Create parent service with test mode
	specService := &SpecDrivenTaskService{
		store:        mockStore,
		controller:   mockController,
		helixAgentID: "test-helix-agent",
		zedAgentPool: []string{"test-zed-agent"},
		testMode:     true,
	}

	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)
	manager.SetTestMode(true)

	ctx := context.Background()
	specTaskID := "test-spec-task"

	// Create test spec task
	specTask := &types.SpecTask{
		ID:                  specTaskID,
		Name:                "Test Authentication System",
		Description:         "Implement user authentication",
		Status:              types.TaskStatusSpecApproved,
		ImplementationPlan:  createTestImplementationPlan(),
		CreatedBy:           "test-user",
		ImplementationAgent: "test-zed-agent",
	}

	config := &types.SpecTaskImplementationSessionsCreateRequest{
		SpecTaskID:         specTaskID,
		ProjectPath:        "/test/project",
		AutoCreateSessions: true,
		WorkspaceConfig: map[string]interface{}{
			"test_config": "value",
		},
	}

	// Mock store expectations
	mockStore.EXPECT().GetSpecTask(ctx, specTaskID).Return(specTask, nil)

	// Note: GetApp and UpdateSpecTask (for workspace config) are only called when
	// zedIntegrationService is not nil. Since it's nil in this test, these aren't called.

	// Expect implementation sessions creation
	expectedWorkSessions := []*types.SpecTaskWorkSession{
		{
			ID:             "ws_1",
			SpecTaskID:     specTaskID,
			HelixSessionID: "session_1",
			Name:           "Database schema migration",
			Phase:          types.SpecTaskPhaseImplementation,
			Status:         types.SpecTaskWorkSessionStatusPending,
		},
		{
			ID:             "ws_2",
			SpecTaskID:     specTaskID,
			HelixSessionID: "session_2",
			Name:           "Authentication API endpoints",
			Phase:          types.SpecTaskPhaseImplementation,
			Status:         types.SpecTaskWorkSessionStatusPending,
		},
	}

	mockStore.EXPECT().CreateImplementationSessions(ctx, specTaskID, config).
		Return(expectedWorkSessions, nil)

	// Note: Since zedIntegrationService is nil in this test, Zed-related operations
	// (CreateZedInstance, CreateZedThread) are skipped by the nil checks in the code.
	// This is intentional for testing the core session creation flow without Zed.

	// Expect spec task status update (always happens)
	mockStore.EXPECT().UpdateSpecTask(ctx, gomock.Any()).Return(nil)

	// Session context service will try to get Zed threads for each work session
	// Since we didn't create Zed threads (zedIntegrationService is nil), these return errors
	for range expectedWorkSessions {
		mockStore.EXPECT().GetSpecTaskZedThreadByWorkSession(ctx, gomock.Any()).
			Return(nil, fmt.Errorf("not found")).AnyTimes()
	}

	// Expect final overview call
	expectedOverview := &types.SpecTaskMultiSessionOverviewResponse{
		SpecTask:         *specTask,
		WorkSessionCount: 2,
		ActiveSessions:   0,
		ZedThreadCount:   2,
		WorkSessions:     []types.SpecTaskWorkSession{*expectedWorkSessions[0], *expectedWorkSessions[1]},
	}
	mockStore.EXPECT().GetSpecTaskMultiSessionOverview(ctx, specTaskID).
		Return(expectedOverview, nil)

	// Execute
	overview, err := manager.CreateImplementationSessions(ctx, specTaskID, config)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, 2, overview.WorkSessionCount)
	assert.Equal(t, 2, overview.ZedThreadCount)
	assert.Equal(t, specTask.Name, overview.SpecTask.Name)
}

func TestSpecTaskMultiSessionManager_SpawnWorkSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)
	manager.SetTestMode(true)

	ctx := context.Background()
	parentSessionID := "parent-session-id"
	specTaskID := "test-spec-task"

	config := &types.SpecTaskWorkSessionSpawnRequest{
		ParentWorkSessionID: parentSessionID,
		Name:                "Debug session",
		Description:         "Debug authentication flow",
		AgentConfig: map[string]interface{}{
			"debug_mode": true,
		},
	}

	// Mock parent work session (no longer includes nested SpecTask)
	_ = &types.SpecTaskWorkSession{
		ID:         parentSessionID,
		SpecTaskID: specTaskID,
		Status:     types.SpecTaskWorkSessionStatusActive,
		Phase:      types.SpecTaskPhaseImplementation,
	}

	specTask := &types.SpecTask{
		ID:            specTaskID,
		ZedInstanceID: "zed-instance-123",
		CreatedBy:     "test-user",
	}

	spawnedWorkSession := &types.SpecTaskWorkSession{
		ID:                  "spawned-session-id",
		SpecTaskID:          specTaskID,
		HelixSessionID:      "spawned-helix-session",
		Name:                config.Name,
		Description:         config.Description,
		Phase:               types.SpecTaskPhaseImplementation,
		Status:              types.SpecTaskWorkSessionStatusPending,
		ParentWorkSessionID: parentSessionID,
		SpawnedBySessionID:  parentSessionID,
	}

	// Mock store expectations
	mockStore.EXPECT().SpawnWorkSession(ctx, parentSessionID, config).
		Return(spawnedWorkSession, nil)
	mockStore.EXPECT().GetSpecTask(ctx, specTaskID).Return(specTask, nil)
	mockStore.EXPECT().CreateSpecTaskZedThread(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, zedThread *types.SpecTaskZedThread) error {
			assert.Equal(t, spawnedWorkSession.ID, zedThread.WorkSessionID)
			assert.Equal(t, specTaskID, zedThread.SpecTaskID)
			return nil
		})

	// Mock detail response
	_ = &types.SpecTaskWorkSessionDetailResponse{
		WorkSession:  *spawnedWorkSession,
		SpecTask:     *specTask,
		HelixSession: &types.Session{ID: "spawned-helix-session"},
	}

	mockStore.EXPECT().GetSpecTaskWorkSession(ctx, spawnedWorkSession.ID).
		Return(spawnedWorkSession, nil)
	mockStore.EXPECT().GetSpecTask(ctx, specTaskID).Return(specTask, nil)
	mockStore.EXPECT().GetSession(ctx, "spawned-helix-session").
		Return(&types.Session{ID: "spawned-helix-session"}, nil)
	mockStore.EXPECT().GetSpecTaskZedThreadByWorkSession(ctx, spawnedWorkSession.ID).
		Return(&types.SpecTaskZedThread{ID: "zed-thread-123"}, nil)
	mockStore.EXPECT().ListSpecTaskImplementationTasks(ctx, specTaskID).
		Return([]*types.SpecTaskImplementationTask{}, nil)
	mockStore.EXPECT().ListSpecTaskWorkSessions(ctx, specTaskID).
		Return([]*types.SpecTaskWorkSession{}, nil)

	// Execute
	detail, err := manager.SpawnWorkSession(ctx, parentSessionID, config)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, config.Name, detail.WorkSession.Name)
	assert.Equal(t, config.Description, detail.WorkSession.Description)
	assert.Equal(t, parentSessionID, detail.WorkSession.ParentWorkSessionID)
	assert.Equal(t, parentSessionID, detail.WorkSession.SpawnedBySessionID)
}

func TestSpecTaskMultiSessionManager_UpdateWorkSessionStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)

	ctx := context.Background()
	workSessionID := "work-session-id"
	specTaskID := "spec-task-id"

	workSession := &types.SpecTaskWorkSession{
		ID:         workSessionID,
		SpecTaskID: specTaskID,
		Status:     types.SpecTaskWorkSessionStatusPending,
		Phase:      types.SpecTaskPhaseImplementation,
	}

	// Mock store expectations
	mockStore.EXPECT().GetSpecTaskWorkSession(ctx, workSessionID).
		Return(workSession, nil)
	mockStore.EXPECT().UpdateSpecTaskWorkSession(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, ws *types.SpecTaskWorkSession) error {
			assert.Equal(t, types.SpecTaskWorkSessionStatusActive, ws.Status)
			assert.NotNil(t, ws.StartedAt)
			return nil
		})

	// Execute
	err := manager.UpdateWorkSessionStatus(ctx, workSessionID, types.SpecTaskWorkSessionStatusActive)

	// Assert
	require.NoError(t, err)
}

func TestSpecTaskMultiSessionManager_UpdateZedThreadStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)

	ctx := context.Background()
	workSessionID := "work-session-id"

	zedThread := &types.SpecTaskZedThread{
		ID:            "zed-thread-id",
		WorkSessionID: workSessionID,
		Status:        types.SpecTaskZedStatusPending,
	}

	workSession := &types.SpecTaskWorkSession{
		ID:     workSessionID,
		Status: types.SpecTaskWorkSessionStatusPending,
	}

	// Mock store expectations
	mockStore.EXPECT().GetSpecTaskZedThreadByWorkSession(ctx, workSessionID).
		Return(zedThread, nil)
	mockStore.EXPECT().UpdateSpecTaskZedThread(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, zt *types.SpecTaskZedThread) error {
			assert.Equal(t, types.SpecTaskZedStatusActive, zt.Status)
			assert.NotNil(t, zt.LastActivityAt)
			return nil
		})

	// Expect work session status update as well
	mockStore.EXPECT().GetSpecTaskWorkSession(ctx, workSessionID).
		Return(workSession, nil)
	mockStore.EXPECT().UpdateSpecTaskWorkSession(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, ws *types.SpecTaskWorkSession) error {
			assert.Equal(t, types.SpecTaskWorkSessionStatusActive, ws.Status)
			return nil
		})

	// Execute
	err := manager.UpdateZedThreadStatus(ctx, workSessionID, types.SpecTaskZedStatusActive)

	// Assert
	require.NoError(t, err)
}

func TestSpecTaskMultiSessionManager_GetMultiSessionOverview(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)

	ctx := context.Background()
	specTaskID := "spec-task-id"

	expectedOverview := &types.SpecTaskMultiSessionOverviewResponse{
		SpecTask: types.SpecTask{
			ID:   specTaskID,
			Name: "Test Task",
		},
		WorkSessionCount:  3,
		ActiveSessions:    2,
		CompletedSessions: 1,
		ZedThreadCount:    3,
	}

	// Mock store expectations
	mockStore.EXPECT().GetSpecTaskMultiSessionOverview(ctx, specTaskID).
		Return(expectedOverview, nil)

	// Execute
	overview, err := manager.GetMultiSessionOverview(ctx, specTaskID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedOverview.WorkSessionCount, overview.WorkSessionCount)
	assert.Equal(t, expectedOverview.ActiveSessions, overview.ActiveSessions)
	assert.Equal(t, expectedOverview.ZedThreadCount, overview.ZedThreadCount)
}

func TestSpecTaskMultiSessionManager_GetSpecTaskProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)

	ctx := context.Background()
	specTaskID := "spec-task-id"

	expectedProgress := &types.SpecTaskProgressResponse{
		SpecTask: types.SpecTask{
			ID:   specTaskID,
			Name: "Test Task",
		},
		OverallProgress: 0.6,
		PhaseProgress: map[types.SpecTaskPhase]float64{
			types.SpecTaskPhasePlanning:       1.0,
			types.SpecTaskPhaseImplementation: 0.6,
		},
		ImplementationProgress: map[int]float64{
			0: 1.0,
			1: 0.5,
			2: 0.0,
		},
	}

	// Mock store expectations
	mockStore.EXPECT().GetSpecTaskProgress(ctx, specTaskID).
		Return(expectedProgress, nil)

	// Execute
	progress, err := manager.GetSpecTaskProgress(ctx, specTaskID)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedProgress.OverallProgress, progress.OverallProgress)
	assert.Equal(t, expectedProgress.PhaseProgress, progress.PhaseProgress)
	assert.Equal(t, expectedProgress.ImplementationProgress, progress.ImplementationProgress)
}

// Test error cases

func TestSpecTaskMultiSessionManager_CreateImplementationSessions_NotApproved(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)

	ctx := context.Background()
	specTaskID := "test-spec-task"

	// Create spec task that is not approved
	specTask := &types.SpecTask{
		ID:     specTaskID,
		Status: types.TaskStatusSpecReview, // Not approved
	}

	config := &types.SpecTaskImplementationSessionsCreateRequest{
		SpecTaskID: specTaskID,
	}

	// Mock store expectations
	mockStore.EXPECT().GetSpecTask(ctx, specTaskID).Return(specTask, nil)

	// Execute
	_, err := manager.CreateImplementationSessions(ctx, specTaskID, config)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be approved")
}

func TestSpecTaskMultiSessionManager_SpawnWorkSession_CannotSpawn(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockController := &controller.Controller{}

	specService := &SpecDrivenTaskService{testMode: true}
	manager := NewSpecTaskMultiSessionManager(
		mockStore,
		mockController,
		specService,
		nil, // ZedIntegrationService
		"test-zed-agent",
	)

	ctx := context.Background()
	parentSessionID := "parent-session-id"

	config := &types.SpecTaskWorkSessionSpawnRequest{
		ParentWorkSessionID: parentSessionID,
		Name:                "Test spawn",
	}

	// Mock parent work session that cannot spawn (completed status)
	_ = &types.SpecTaskWorkSession{
		ID:     parentSessionID,
		Status: types.SpecTaskWorkSessionStatusCompleted, // Cannot spawn
		Phase:  types.SpecTaskPhaseImplementation,
	}

	// Mock store expectations
	mockStore.EXPECT().SpawnWorkSession(ctx, parentSessionID, config).
		Return(nil, assert.AnError)

	// Execute
	_, err := manager.SpawnWorkSession(ctx, parentSessionID, config)

	// Assert
	require.Error(t, err)
}

// Helper functions

func createTestImplementationPlan() string {
	return `# Implementation Plan

## Task 1: Database schema migration
- Description: Create user authentication tables
- Effort: Small
- Dependencies: None

## Task 2: Authentication API endpoints
- Description: Implement login/logout endpoints
- Effort: Medium
- Dependencies: Task 1

## Task 3: Frontend integration
- Description: Connect UI to authentication API
- Effort: Medium
- Dependencies: Task 2`
}
