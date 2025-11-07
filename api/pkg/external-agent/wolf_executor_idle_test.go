package external_agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestCleanupIdleExternalAgents_NoIdleAgents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockStore := store.NewMockStore(ctrl)
	mockWolfClient := NewMockWolfClientInterface(ctrl)

	executor := &WolfExecutor{
		store:      mockStore,
		wolfClient: mockWolfClient,
	}

	// Mock: No idle agents
	mockStore.EXPECT().
		GetIdleExternalAgents(ctx, gomock.Any(), []string{"spectask"}).
		Return([]*types.ExternalAgentActivity{}, nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: No cleanup actions taken (no calls to Wolf client expected)
	// gomock will verify no unexpected calls were made
}

func TestCleanupIdleExternalAgents_TerminatesIdleAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockStore := store.NewMockStore(ctrl)
	mockWolfClient := NewMockWolfClientInterface(ctrl)

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

	// Setup expectations in order
	mockStore.EXPECT().
		GetIdleExternalAgents(ctx, gomock.Any(), []string{"spectask"}).
		Return([]*types.ExternalAgentActivity{idleActivity}, nil)

	mockWolfClient.EXPECT().
		RemoveApp(ctx, "wolf_idle_456").
		Return(nil)

	mockStore.EXPECT().
		GetSpecTaskExternalAgentByID(ctx, "zed-spectask-idle123").
		Return(idleAgent, nil)

	mockStore.EXPECT().
		UpdateSpecTaskExternalAgent(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
			if agent.Status != "terminated" {
				t.Errorf("expected agent status to be 'terminated', got '%s'", agent.Status)
			}
			return nil
		})

	mockStore.EXPECT().
		GetSession(ctx, "ses_001").
		Return(session1, nil)

	mockStore.EXPECT().
		UpdateSession(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, session types.Session) (*types.Session, error) {
			if session.Metadata.ExternalAgentStatus != "terminated_idle" {
				t.Errorf("expected session external_agent_status to be 'terminated_idle', got '%s'", session.Metadata.ExternalAgentStatus)
			}
			return &types.Session{}, nil
		})

	mockStore.EXPECT().
		GetSession(ctx, "ses_002").
		Return(session2, nil)

	mockStore.EXPECT().
		UpdateSession(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, session types.Session) (*types.Session, error) {
			if session.Metadata.ExternalAgentStatus != "terminated_idle" {
				t.Errorf("expected session external_agent_status to be 'terminated_idle', got '%s'", session.Metadata.ExternalAgentStatus)
			}
			return &types.Session{}, nil
		})

	mockStore.EXPECT().
		DeleteExternalAgentActivity(ctx, "zed-spectask-idle123").
		Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// All verifications are handled by gomock expectations
}

func TestCleanupIdleExternalAgents_WolfRemovalFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockStore := store.NewMockStore(ctrl)
	mockWolfClient := NewMockWolfClientInterface(ctrl)

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

	// Setup expectations
	mockStore.EXPECT().
		GetIdleExternalAgents(ctx, gomock.Any(), []string{"spectask"}).
		Return([]*types.ExternalAgentActivity{idleActivity}, nil)

	// Wolf client fails to remove app
	mockWolfClient.EXPECT().
		RemoveApp(ctx, "wolf_fail_789").
		Return(fmt.Errorf("wolf connection error"))

	// But cleanup should continue anyway
	mockStore.EXPECT().
		GetSpecTaskExternalAgentByID(ctx, "zed-spectask-wolf-fail").
		Return(idleAgent, nil)

	mockStore.EXPECT().
		UpdateSpecTaskExternalAgent(ctx, gomock.Any()).
		Return(nil)

	mockStore.EXPECT().
		GetSession(ctx, "ses_fail_001").
		Return(session, nil)

	mockStore.EXPECT().
		UpdateSession(ctx, gomock.Any()).
		Return(&types.Session{}, nil)

	mockStore.EXPECT().
		DeleteExternalAgentActivity(ctx, "zed-spectask-wolf-fail").
		Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Cleanup continues even though Wolf removal failed
	// All verifications are handled by gomock expectations
}

func TestCleanupIdleExternalAgents_MultipleAgents(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	mockStore := store.NewMockStore(ctrl)
	mockWolfClient := NewMockWolfClientInterface(ctrl)

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

	// Setup expectations
	mockStore.EXPECT().
		GetIdleExternalAgents(ctx, gomock.Any(), []string{"spectask"}).
		Return(idleActivities, nil)

	// Expectations for first agent
	mockWolfClient.EXPECT().
		RemoveApp(ctx, "wolf_1").
		Return(nil)

	mockStore.EXPECT().
		GetSpecTaskExternalAgentByID(ctx, "zed-spectask-idle1").
		Return(agent1, nil)

	mockStore.EXPECT().
		UpdateSpecTaskExternalAgent(ctx, gomock.Any()).
		Return(nil)

	mockStore.EXPECT().
		DeleteExternalAgentActivity(ctx, "zed-spectask-idle1").
		Return(nil)

	// Expectations for second agent
	mockWolfClient.EXPECT().
		RemoveApp(ctx, "wolf_2").
		Return(nil)

	mockStore.EXPECT().
		GetSpecTaskExternalAgentByID(ctx, "zed-spectask-idle2").
		Return(agent2, nil)

	mockStore.EXPECT().
		UpdateSpecTaskExternalAgent(ctx, gomock.Any()).
		Return(nil)

	mockStore.EXPECT().
		DeleteExternalAgentActivity(ctx, "zed-spectask-idle2").
		Return(nil)

	// Execute
	executor.cleanupIdleExternalAgents(ctx)

	// Verify: Both agents terminated
	// All verifications are handled by gomock expectations
}
