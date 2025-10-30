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
		WolfAppID:       "wolf_app_456",
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
	assert.Equal(t, agent.WolfAppID, retrieved.WolfAppID)
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
		WolfAppID:       "wolf_app_789",
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
		ID:           "zed-spectask-list1",
		SpecTaskID:   "spec_list1",
		WolfAppID:    "wolf_1",
		WorkspaceDir: "/workspaces/spectasks/spec_list1/work",
		Status:       "running",
		Created:      time.Now(),
		LastActivity: time.Now(),
		UserID:       "user_alice",
	}

	agent2 := &types.SpecTaskExternalAgent{
		ID:           "zed-spectask-list2",
		SpecTaskID:   "spec_list2",
		WolfAppID:    "wolf_2",
		WorkspaceDir: "/workspaces/spectasks/spec_list2/work",
		Status:       "running",
		Created:      time.Now(),
		LastActivity: time.Now(),
		UserID:       "user_alice",
	}

	agent3 := &types.SpecTaskExternalAgent{
		ID:           "zed-spectask-list3",
		SpecTaskID:   "spec_list3",
		WolfAppID:    "wolf_3",
		WorkspaceDir: "/workspaces/spectasks/spec_list3/work",
		Status:       "running",
		Created:      time.Now(),
		LastActivity: time.Now(),
		UserID:       "user_bob",
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

func TestExternalAgentActivity_UpsertAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	ctx := context.Background()
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create activity
	activity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-activity1",
		SpecTaskID:      "spec_activity1",
		LastInteraction: time.Now(),
		AgentType:       "spectask",
		WolfAppID:       "wolf_activity_1",
		WorkspaceDir:    "/workspaces/spectasks/spec_activity1/work",
		UserID:          "user_789",
	}

	// Upsert (insert)
	err := store.UpsertExternalAgentActivity(ctx, activity)
	require.NoError(t, err)

	// Get
	retrieved, err := store.GetExternalAgentActivity(ctx, "zed-spectask-activity1")
	require.NoError(t, err)
	assert.Equal(t, activity.ExternalAgentID, retrieved.ExternalAgentID)
	assert.Equal(t, activity.SpecTaskID, retrieved.SpecTaskID)
	assert.Equal(t, "spectask", retrieved.AgentType)

	// Upsert again (update)
	time.Sleep(100 * time.Millisecond)
	activity.SpecTaskID = "spec_activity1_updated"
	err = store.UpsertExternalAgentActivity(ctx, activity)
	require.NoError(t, err)

	// Verify update
	retrieved2, err := store.GetExternalAgentActivity(ctx, "zed-spectask-activity1")
	require.NoError(t, err)
	assert.Equal(t, "spec_activity1_updated", retrieved2.SpecTaskID)
	assert.True(t, retrieved2.LastInteraction.After(retrieved.LastInteraction))
}

func TestExternalAgentActivity_GetIdleAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	ctx := context.Background()
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create recent activity (not idle)
	recentActivity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-recent",
		SpecTaskID:      "spec_recent",
		LastInteraction: time.Now(),
		AgentType:       "spectask",
		WolfAppID:       "wolf_recent",
		WorkspaceDir:    "/workspaces/spectasks/spec_recent/work",
		UserID:          "user_recent",
	}
	require.NoError(t, store.UpsertExternalAgentActivity(ctx, recentActivity))

	// Create old activity (idle for >30min)
	oldActivity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-old",
		SpecTaskID:      "spec_old",
		LastInteraction: time.Now().Add(-35 * time.Minute),
		AgentType:       "spectask",
		WolfAppID:       "wolf_old",
		WorkspaceDir:    "/workspaces/spectasks/spec_old/work",
		UserID:          "user_old",
	}

	// Manually insert with old timestamp (bypassing Upsert which sets time.Now())
	// Cast to *PostgresStore to access GetDB() method for testing
	pgStore, ok := store.(*PostgresStore)
	require.True(t, ok, "store must be *PostgresStore for this test")

	err := pgStore.GetDB().WithContext(ctx).Exec(`
		INSERT INTO external_agent_activity (external_agent_id, spec_task_id, last_interaction, agent_type, wolf_app_id, workspace_dir, user_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, oldActivity.ExternalAgentID, oldActivity.SpecTaskID, oldActivity.LastInteraction, oldActivity.AgentType, oldActivity.WolfAppID, oldActivity.WorkspaceDir, oldActivity.UserID).Error
	require.NoError(t, err)

	// Get idle agents (>30min)
	cutoff := time.Now().Add(-30 * time.Minute)
	idleAgents, err := store.GetIdleExternalAgents(ctx, cutoff, []string{"spectask"})
	require.NoError(t, err)

	// Should find the old one but not the recent one
	foundOld := false
	foundRecent := false
	for _, agent := range idleAgents {
		if agent.ExternalAgentID == "zed-spectask-old" {
			foundOld = true
		}
		if agent.ExternalAgentID == "zed-spectask-recent" {
			foundRecent = true
		}
	}

	assert.True(t, foundOld, "Should find idle agent older than 30min")
	assert.False(t, foundRecent, "Should not find recent agent")
}

func TestExternalAgentActivity_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	ctx := context.Background()
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Create activity
	activity := &types.ExternalAgentActivity{
		ExternalAgentID: "zed-spectask-delete-test",
		SpecTaskID:      "spec_delete_test",
		LastInteraction: time.Now(),
		AgentType:       "spectask",
		WolfAppID:       "wolf_delete",
		WorkspaceDir:    "/workspaces/spectasks/spec_delete_test/work",
		UserID:          "user_delete",
	}

	err := store.UpsertExternalAgentActivity(ctx, activity)
	require.NoError(t, err)

	// Delete
	err = store.DeleteExternalAgentActivity(ctx, "zed-spectask-delete-test")
	require.NoError(t, err)

	// Verify deleted
	_, err = store.GetExternalAgentActivity(ctx, "zed-spectask-delete-test")
	assert.Error(t, err) // Should not be found
}

// setupTestStore creates a test store instance
// Note: This assumes a test database is available
func setupTestStore(t *testing.T) (Store, func()) {
	// TODO: Implement test database setup
	// For now, this is a placeholder that would need actual database connection
	t.Skip("Test database setup not implemented yet")
	return nil, func() {}
}
