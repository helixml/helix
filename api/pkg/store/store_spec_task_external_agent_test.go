package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpecTaskExternalAgent_CreateAndGet(t *testing.T) {
	// This test requires a real database connection
	// Skip if running in CI without database
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	ctx := context.Background()
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create test external agent
	agent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-test123",
		SpecTaskID:      "spec_test123",
		ContainerAppID:  "wolf_app_456",
		WorkspaceDir:    "/workspaces/spectasks/spec_test123/work",
		HelixSessionIDs: []string{"ses_001", "ses_002"},
		ZedThreadIDs:    []string{"thread_001", "thread_002"},
		Status:          "running",
		Created:         time.Now(),
		LastActivity:    time.Now(),
		UserID:          "user_123",
	}

	// Test Create
	err := store.CreateSpecTaskExternalAgent(ctx, agent)
	require.NoError(t, err)

	// Test Get by SpecTaskID
	retrieved, err := store.GetSpecTaskExternalAgent(ctx, "spec_test123")
	require.NoError(t, err)
	assert.Equal(t, agent.ID, retrieved.ID)
	assert.Equal(t, agent.SpecTaskID, retrieved.SpecTaskID)
	assert.Equal(t, agent.ContainerAppID, retrieved.ContainerAppID)
	assert.Equal(t, agent.Status, retrieved.Status)
	assert.Equal(t, 2, len(retrieved.HelixSessionIDs))
	assert.Contains(t, retrieved.HelixSessionIDs, "ses_001")
	assert.Contains(t, retrieved.HelixSessionIDs, "ses_002")

	// Test Get by AgentID
	retrieved2, err := store.GetSpecTaskExternalAgentByID(ctx, "zed-spectask-test123")
	require.NoError(t, err)
	assert.Equal(t, agent.ID, retrieved2.ID)
}

func TestSpecTaskExternalAgent_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	ctx := context.Background()
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create initial agent
	agent := &types.SpecTaskExternalAgent{
		ID:              "zed-spectask-update-test",
		SpecTaskID:      "spec_update_test",
		ContainerAppID:  "wolf_app_789",
		WorkspaceDir:    "/workspaces/spectasks/spec_update_test/work",
		HelixSessionIDs: []string{"ses_001"},
		ZedThreadIDs:    []string{},
		Status:          "running",
		Created:         time.Now(),
		LastActivity:    time.Now(),
		UserID:          "user_456",
	}

	err := store.CreateSpecTaskExternalAgent(ctx, agent)
	require.NoError(t, err)

	// Update agent - add new session
	agent.HelixSessionIDs = append(agent.HelixSessionIDs, "ses_002")
	agent.ZedThreadIDs = append(agent.ZedThreadIDs, "thread_002")
	agent.Status = "terminated"

	err = store.UpdateSpecTaskExternalAgent(ctx, agent)
	require.NoError(t, err)

	// Verify update
	retrieved, err := store.GetSpecTaskExternalAgent(ctx, "spec_update_test")
	require.NoError(t, err)
	assert.Equal(t, "terminated", retrieved.Status)
	assert.Equal(t, 2, len(retrieved.HelixSessionIDs))
	assert.Contains(t, retrieved.HelixSessionIDs, "ses_002")
}

func TestSpecTaskExternalAgent_List(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	ctx := context.Background()
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create multiple agents for different users
	agent1 := &types.SpecTaskExternalAgent{
		ID:             "zed-spectask-list1",
		SpecTaskID:     "spec_list1",
		ContainerAppID: "wolf_1",
		WorkspaceDir:   "/workspaces/spectasks/spec_list1/work",
		Status:         "running",
		Created:        time.Now(),
		LastActivity:   time.Now(),
		UserID:         "user_alice",
	}

	agent2 := &types.SpecTaskExternalAgent{
		ID:             "zed-spectask-list2",
		SpecTaskID:     "spec_list2",
		ContainerAppID: "wolf_2",
		WorkspaceDir:   "/workspaces/spectasks/spec_list2/work",
		Status:         "running",
		Created:        time.Now(),
		LastActivity:   time.Now(),
		UserID:         "user_alice",
	}

	agent3 := &types.SpecTaskExternalAgent{
		ID:             "zed-spectask-list3",
		SpecTaskID:     "spec_list3",
		ContainerAppID: "wolf_3",
		WorkspaceDir:   "/workspaces/spectasks/spec_list3/work",
		Status:         "running",
		Created:        time.Now(),
		LastActivity:   time.Now(),
		UserID:         "user_bob",
	}

	require.NoError(t, store.CreateSpecTaskExternalAgent(ctx, agent1))
	require.NoError(t, store.CreateSpecTaskExternalAgent(ctx, agent2))
	require.NoError(t, store.CreateSpecTaskExternalAgent(ctx, agent3))

	// List all agents for user_alice
	aliceAgents, err := store.ListSpecTaskExternalAgents(ctx, "user_alice")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(aliceAgents), 2) // At least 2 from this test

	// List all agents (no user filter)
	allAgents, err := store.ListSpecTaskExternalAgents(ctx, "")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allAgents), 3) // At least 3 from this test
}

// setupTestStore creates a test store instance
// Note: This assumes a test database is available
func setupTestStore(t *testing.T) (Store, func()) {
	// TODO: Implement test database setup
	// For now, this is a placeholder that would need actual database connection
	t.Skip("Test database setup not implemented yet")
	return nil, func() {}
}
