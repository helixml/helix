package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TestPostgresStore_ListIdleDesktops_ReturnsIdleDesktop verifies that a running
// desktop whose last interaction is older than the threshold is returned.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_ReturnsIdleDesktop() {
	ctx := context.Background()
	containerID := "container-idle-" + system.GenerateUUID()

	// Both session and interaction must be older than the idle threshold
	oldTime := time.Now().Add(-2 * time.Hour)
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: oldTime,
		Updated: oldTime,
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	// Interaction updated 2 hours ago — outside the 1-hour threshold
	interaction := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       "user_id",
		Created:      oldTime,
		Updated:      oldTime,
	}
	_, err = suite.db.CreateInteraction(ctx, interaction)
	suite.NoError(err)

	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)

	found := false
	for _, s := range results {
		if s.ID == session.ID {
			found = true
			break
		}
	}
	suite.True(found, "expected idle desktop session to be returned")
}

// TestPostgresStore_ListIdleDesktops_SkipsRecentInteraction verifies that a
// desktop with a recent interaction is not considered idle.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsRecentInteraction() {
	ctx := context.Background()
	containerID := "container-recent-" + system.GenerateUUID()

	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	// Interaction updated just 5 minutes ago — within the threshold
	recentTime := time.Now().Add(-5 * time.Minute)
	interaction := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       "user_id",
		Created:      recentTime,
		Updated:      recentTime,
	}
	_, err = suite.db.CreateInteraction(ctx, interaction)
	suite.NoError(err)

	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)

	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "desktop with recent interaction must not be returned")
	}
}

// TestPostgresStore_ListIdleDesktops_SkipsStoppedDesktop verifies that a
// desktop not in "running" status is excluded.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsStoppedDesktop() {
	ctx := context.Background()
	containerID := "container-stopped-" + system.GenerateUUID()

	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now().Add(-2 * time.Hour),
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "stopped",
			DevContainerID:      containerID,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)

	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "stopped desktop must not be returned")
	}
}

// TestPostgresStore_ListIdleDesktops_SkipsKeepAliveTask verifies that a
// desktop whose parent spec task has keep_alive=true is excluded from idle results.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsKeepAliveTask() {
	ctx := context.Background()
	containerID := "container-keepalive-" + system.GenerateUUID()

	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(),
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	// Interaction updated 2 hours ago — would normally be idle
	oldTime := time.Now().Add(-2 * time.Hour)
	interaction := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       "user_id",
		Created:      oldTime,
		Updated:      oldTime,
	}
	_, err = suite.db.CreateInteraction(ctx, interaction)
	suite.NoError(err)

	// Create a spec task with keep_alive=true pointing at this session
	specTask := &types.SpecTask{
		ID:                "st_keepalive_" + system.GenerateUUID(),
		ProjectID:         "proj_test_" + system.GenerateUUID(),
		UserID:            "user_id",
		Name:              "keep-alive-test",
		PlanningSessionID: session.ID,
		KeepAlive:         true,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}
	err = suite.db.CreateSpecTask(ctx, specTask)
	suite.NoError(err)
	suite.T().Cleanup(func() { _ = suite.db.gdb.WithContext(ctx).Delete(specTask).Error })

	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)

	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "desktop with keep_alive spec task must not be returned")
	}
}

// TestPostgresStore_ListIdleDesktops_SkipsRecentSessionWithNoInteractions
// verifies that a brand-new desktop (no interactions, recently updated) is
// not considered idle — the session's own updated timestamp is used as the
// activity marker when there are no interactions.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsRecentSessionWithNoInteractions() {
	ctx := context.Background()
	containerID := "container-nointeractions-" + system.GenerateUUID()

	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: time.Now(),
		Updated: time.Now(), // just updated — not idle
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)

	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "recently created desktop with no interactions must not be returned")
	}
}

// TestPostgresStore_ListIdleDesktops_SkipsRestartedSessionWithOldInteractions
// verifies that restarting a stopped session (via UpdateSession) bumps the
// Updated timestamp, preventing the idle checker from killing it immediately.
// This is the exact scenario from the bug: a session with stale interactions
// gets restarted → UpdateSession must bump s.updated so GREATEST picks the
// fresh timestamp.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsRestartedSessionWithOldInteractions() {
	ctx := context.Background()
	containerID := "container-restarted-" + system.GenerateUUID()

	// Session created and last updated 3 hours ago (idle)
	oldTime := time.Now().Add(-3 * time.Hour)
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: oldTime,
		Updated: oldTime,
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "stopped",
			DevContainerID:      containerID,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	// Old interaction from 3 hours ago
	interaction := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       "user_id",
		Created:      oldTime,
		Updated:      oldTime,
	}
	_, err = suite.db.CreateInteraction(ctx, interaction)
	suite.NoError(err)

	// Simulate restart: read-modify-write via UpdateSession (like StartDesktop does)
	dbSession, err := suite.db.GetSession(ctx, session.ID)
	suite.NoError(err)
	dbSession.Metadata.ExternalAgentStatus = "running"
	_, err = suite.db.UpdateSession(ctx, *dbSession)
	suite.NoError(err)

	// The session was just restarted — it must NOT be considered idle
	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)

	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "just-restarted desktop must not be returned as idle")
	}
}

// TestPostgresStore_ListIdleDesktops_LongOverrideKeepsAlive verifies that a
// desktop whose instance profile overrides the idle timeout to a LONGER value
// than the deployment default stays up past the default.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_LongOverrideKeepsAlive() {
	ctx := context.Background()
	containerID := "container-long-override-" + system.GenerateUUID()

	oldTime := time.Now().Add(-2 * time.Hour)
	profile := types.BotInstanceProfile{IdleTimeoutSeconds: 6 * 3600} // 6h > the 1h default
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: oldTime,
		Updated: oldTime,
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
			BotInstance:         &profile,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	interaction := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       "user_id",
		Created:      oldTime,
		Updated:      oldTime,
	}
	_, err = suite.db.CreateInteraction(ctx, interaction)
	suite.NoError(err)

	// 2h idle vs the 1h default: without the override this would be returned.
	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)
	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "6h override must keep a 2h-idle desktop alive")
	}
}

// TestPostgresStore_ListIdleDesktops_ShortOverrideStopsEarly verifies that an
// override SHORTER than the deployment default stops the desktop early.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_ShortOverrideStopsEarly() {
	ctx := context.Background()
	containerID := "container-short-override-" + system.GenerateUUID()

	oldTime := time.Now().Add(-20 * time.Minute)
	profile := types.BotInstanceProfile{IdleTimeoutSeconds: 300} // 5m < the 1h default
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: oldTime,
		Updated: oldTime,
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
			BotInstance:         &profile,
		},
	}
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	// 20m idle vs the 1h default: only the 5m override makes it idle.
	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)
	found := false
	for _, s := range results {
		if s.ID == session.ID {
			found = true
		}
	}
	suite.True(found, "5m override must stop a 20m-idle desktop despite the 1h default")
}

// TestPostgresStore_ListIdleDesktops_GarbageOverrideIgnored verifies that a
// non-numeric override value falls back to the deployment default instead of
// erroring or changing the threshold.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_GarbageOverrideIgnored() {
	ctx := context.Background()
	containerID := "container-garbage-override-" + system.GenerateUUID()

	oldTime := time.Now().Add(-2 * time.Hour)
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "user_id",
		Created: oldTime,
		Updated: oldTime,
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      containerID,
		},
	}
	// Write a garbage override straight into the metadata jsonb.
	_, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	interaction := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       "user_id",
		Created:      oldTime,
		Updated:      oldTime,
	}
	_, err = suite.db.CreateInteraction(ctx, interaction)
	suite.NoError(err)

	raw, err := json.Marshal(types.SessionMetadata{ExternalAgentStatus: "running", DevContainerID: containerID, BotInstance: &types.BotInstanceProfile{}})
	suite.NoError(err)
	var metadata map[string]any
	suite.NoError(json.Unmarshal(raw, &metadata))
	metadata["bot_instance"] = map[string]any{"idle_timeout_seconds": "soon"}
	bts, err := json.Marshal(metadata)
	suite.NoError(err)
	err = suite.db.gdb.WithContext(ctx).Exec("UPDATE sessions SET config = ? WHERE id = ?", string(bts), session.ID).Error
	suite.NoError(err)

	now := time.Now().UTC()
	results, err := suite.db.ListIdleDesktops(ctx, now, 1*time.Hour)
	suite.NoError(err)
	found := false
	for _, s := range results {
		if s.ID == session.ID {
			found = true
		}
	}
	suite.True(found, "garbage override must fall back to the default (2h idle > 1h)")
}
