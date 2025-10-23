package external_agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockStoreForWolf implements store methods needed by Wolf executor
type MockStoreForWolf struct {
	mock.Mock
}

func (m *MockStoreForWolf) GetIdleExternalAgents(ctx context.Context, cutoff time.Time, agentTypes []string) ([]*types.ExternalAgentActivity, error) {
	args := m.Called(ctx, cutoff, agentTypes)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*types.ExternalAgentActivity), args.Error(1)
}

func (m *MockStoreForWolf) GetSpecTaskExternalAgentByID(ctx context.Context, agentID string) (*types.SpecTaskExternalAgent, error) {
	args := m.Called(ctx, agentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.SpecTaskExternalAgent), args.Error(1)
}

func (m *MockStoreForWolf) UpdateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	args := m.Called(ctx, agent)
	return args.Error(0)
}

func (m *MockStoreForWolf) GetSession(ctx context.Context, id string) (*types.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockStoreForWolf) UpdateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	args := m.Called(ctx, session)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*types.Session), args.Error(1)
}

func (m *MockStoreForWolf) DeleteExternalAgentActivity(ctx context.Context, agentID string) error {
	args := m.Called(ctx, agentID)
	return args.Error(0)
}

// MockWolfClient mocks the Wolf client
type MockWolfClient struct {
	mock.Mock
}

func (m *MockWolfClient) RemoveApp(ctx context.Context, appID string) error {
	args := m.Called(ctx, appID)
	return args.Error(0)
}

func TestCleanupIdleExternalAgents_NoIdleAgents(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreForWolf)
	mockWolfClient := new(MockWolfClient)

	executor := &WolfExecutor{
		store:      mockStore,
		wolfClient: mockWolfClient,
	}

	cutoff := time.Now().Add(-30 * time.Minute)

	// Mock: No idle agents
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask"}).
		Return([]*types.ExternalAgentActivity{}, nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: No cleanup actions taken
	mockWolfClient.AssertNotCalled(t, "RemoveApp")
	mockStore.AssertExpectations(t)
}

func TestCleanupIdleExternalAgents_TerminatesIdleAgent(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreForWolf)
	mockWolfClient := new(MockWolfClient)

	executor := &WolfExecutor{
		store:      mockStore,
		wolfClient: mockWolfClient,
	}

	// Idle agent activity
	idleActivity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-idle123",
		SpecTaskID:      "spec_idle123",
		LastInteraction: time.Now().Add(-35 * time.Minute),
		AgentType:       "spectask",
		WolfAppID:       "wolf_idle_456",
		WorkspaceDir:    "/workspaces/spectasks/spec_idle123/work",
		UserID:          "user_idle",
	}

	idleAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-idle123",
		SpecTaskID:      "spec_idle123",
		WolfAppID:       "wolf_idle_456",
		HelixSessionIDs: []string{"ses_001", "ses_002"},
		Status:          "running",
	}

	session1 := &types.Session{
		ID:    "ses_001",
		Owner: "user_idle",
		Metadata: types.SessionMetadata{
			SpecTaskID: "spec_idle123",
		},
	}

	session2 := &types.Session{
		ID:    "ses_002",
		Owner: "user_idle",
		Metadata: types.SessionMetadata{
			SpecTaskID: "spec_idle123",
		},
	}

	// Setup mocks
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask"}).
		Return([]*types.ExternalAgentActivity{idleActivity}, nil)

	mockWolfClient.On("RemoveApp", ctx, "wolf_idle_456").Return(nil)

	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-idle123").
		Return(idleAgent, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.MatchedBy(func(agent *types.SpecTaskExternalAgent) bool {
		return agent.Status == "terminated"
	})).Return(nil)

	mockStore.On("GetSession", ctx, "ses_001").Return(session1, nil)
	mockStore.On("GetSession", ctx, "ses_002").Return(session2, nil)

	mockStore.On("UpdateSession", ctx, mock.MatchedBy(func(session types.Session) bool {
		return session.Metadata.ExternalAgentStatus == "terminated_idle"
	})).Return(&types.Session{}, nil).Times(2)

	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-idle123").Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Wolf app removed
	mockWolfClient.AssertCalled(t, "RemoveApp", ctx, "wolf_idle_456")

	// Verify: Agent status updated to terminated
	mockStore.AssertCalled(t, "UpdateSpecTaskExternalAgent", ctx, mock.Anything)

	// Verify: All sessions updated
	mockStore.AssertNumberOfCalls(t, "UpdateSession", 2)

	// Verify: Activity record deleted
	mockStore.AssertCalled(t, "DeleteExternalAgentActivity", ctx, "zed-spectask-idle123")
}

func TestCleanupIdleExternalAgents_WolfRemovalFails(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreForWolf)
	mockWolfClient := new(MockWolfClient)

	executor := &WolfExecutor{
		store:      mockStore,
		wolfClient: mockWolfClient,
	}

	idleActivity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-wolf-fail",
		SpecTaskID:      "spec_wolf_fail",
		LastInteraction: time.Now().Add(-40 * time.Minute),
		AgentType:       "spectask",
		WolfAppID:       "wolf_fail_789",
		WorkspaceDir:    "/workspaces/spectasks/spec_wolf_fail/work",
		UserID:          "user_fail",
	}

	idleAgent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-wolf-fail",
		SpecTaskID:      "spec_wolf_fail",
		WolfAppID:       "wolf_fail_789",
		HelixSessionIDs: []string{"ses_fail_001"},
		Status:          "running",
	}

	session := &types.Session{
		ID:    "ses_fail_001",
		Owner: "user_fail",
	}

	// Setup mocks
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask"}).
		Return([]*types.ExternalAgentActivity{idleActivity}, nil)

	// Wolf client fails to remove app
	mockWolfClient.On("RemoveApp", ctx, "wolf_fail_789").
		Return(fmt.Errorf("wolf connection error"))

	// But cleanup should continue anyway
	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-wolf-fail").
		Return(idleAgent, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil)
	mockStore.On("GetSession", ctx, "ses_fail_001").Return(session, nil)
	mockStore.On("UpdateSession", ctx, mock.Anything).Return(&types.Session{}, nil)
	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-wolf-fail").Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Continues with cleanup even though Wolf removal failed
	mockStore.AssertCalled(t, "UpdateSpecTaskExternalAgent", ctx, mock.Anything)
	mockStore.AssertCalled(t, "DeleteExternalAgentActivity", ctx, "zed-spectask-wolf-fail")
}

func TestCleanupIdleExternalAgents_MultipleAgents(t *testing.T) {
	ctx := context.Background()
	mockStore := new(MockStoreForWolf)
	mockWolfClient := new(MockWolfClient)

	executor := &WolfExecutor{
		store:      mockStore,
		wolfClient: mockWolfClient,
	}

	// Multiple idle agents
	idleActivities := []*types.ExternalAgentActivity{
		{
			ExternalAgentID: "zed-spectask-idle1",
			SpecTaskID:      "spec_idle1",
			LastInteraction: time.Now().Add(-31 * time.Minute),
			AgentType:       "spectask",
			WolfAppID:       "wolf_1",
			WorkspaceDir:    "/workspaces/spectasks/spec_idle1/work",
			UserID:          "user1",
		},
		{
			ExternalAgentID: "zed-spectask-idle2",
			SpecTaskID:      "spec_idle2",
			LastInteraction: time.Now().Add(-45 * time.Minute),
			AgentType:       "spectask",
			WolfAppID:       "wolf_2",
			WorkspaceDir:    "/workspaces/spectasks/spec_idle2/work",
			UserID:          "user2",
		},
	}

	agent1 := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-idle1",
		WolfAppID:       "wolf_1",
		HelixSessionIDs: []string{},
		Status:          "running",
	}

	agent2 := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-idle2",
		WolfAppID:       "wolf_2",
		HelixSessionIDs: []string{},
		Status:          "running",
	}

	// Setup mocks
	mockStore.On("GetIdleExternalAgents", ctx, mock.Anything, []string{"spectask"}).
		Return(idleActivities, nil)

	mockWolfClient.On("RemoveApp", ctx, "wolf_1").Return(nil)
	mockWolfClient.On("RemoveApp", ctx, "wolf_2").Return(nil)

	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-idle1").Return(agent1, nil)
	mockStore.On("GetSpecTaskExternalAgentByID", ctx, "zed-spectask-idle2").Return(agent2, nil)

	mockStore.On("UpdateSpecTaskExternalAgent", ctx, mock.Anything).Return(nil).Times(2)
	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-idle1").Return(nil)
	mockStore.On("DeleteExternalAgentActivity", ctx, "zed-spectask-idle2").Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Both agents terminated
	mockWolfClient.AssertCalled(t, "RemoveApp", ctx, "wolf_1")
	mockWolfClient.AssertCalled(t, "RemoveApp", ctx, "wolf_2")
	mockStore.AssertNumberOfCalls(t, "DeleteExternalAgentActivity", 2)
}
