package store

import (
	"context"
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

	idleSince := time.Now().Add(-1 * time.Hour)
	results, err := suite.db.ListIdleDesktops(ctx, idleSince)
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

	idleSince := time.Now().Add(-1 * time.Hour)
	results, err := suite.db.ListIdleDesktops(ctx, idleSince)
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

	idleSince := time.Now().Add(-1 * time.Hour)
	results, err := suite.db.ListIdleDesktops(ctx, idleSince)
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

	idleSince := time.Now().Add(-1 * time.Hour)
	results, err := suite.db.ListIdleDesktops(ctx, idleSince)
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

	idleSince := time.Now().Add(-1 * time.Hour)
	results, err := suite.db.ListIdleDesktops(ctx, idleSince)
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
	idleSince := time.Now().Add(-1 * time.Hour)
	results, err := suite.db.ListIdleDesktops(ctx, idleSince)
	suite.NoError(err)

	for _, s := range results {
		suite.NotEqual(session.ID, s.ID, "just-restarted desktop must not be returned as idle")
	}
}

// TestPostgresStore_ListIdleDesktops_SkipsGoldenBuild verifies that a golden
// build session is never returned as idle. It never has interactions, so
// without the exclusion the idle checker killed every golden build that ran
// longer than HELIX_DESKTOP_IDLE_TIMEOUT. An otherwise identical non-golden
// desktop is returned, proving the exclusion is what skips it.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsGoldenBuild() {
	ctx := context.Background()
	oldTime := time.Now().Add(-2 * time.Hour)

	newDesktop := func(goldenBuild bool) types.Session {
		session := types.Session{
			ID:      system.GenerateSessionID(),
			Owner:   "user_id",
			Created: oldTime,
			Updated: oldTime,
			Metadata: types.SessionMetadata{
				ExternalAgentStatus: "running",
				DevContainerID:      "container-golden-" + system.GenerateUUID(),
				GoldenBuild:         goldenBuild,
			},
		}
		_, err := suite.db.CreateSession(ctx, session)
		suite.NoError(err)
		suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
		return session
	}
	golden := newDesktop(true)
	plain := newDesktop(false)

	results, err := suite.db.ListIdleDesktops(ctx, time.Now().Add(-1*time.Hour))
	suite.NoError(err)

	returned := map[string]bool{}
	for _, s := range results {
		returned[s.ID] = true
	}
	suite.False(returned[golden.ID], "golden build session must never be returned as idle")
	suite.True(returned[plain.ID], "non-golden desktop with the same activity should be idle")
}

// TestPostgresStore_ListIdleDesktops_SkipsKeepAliveOrgBot verifies that an
// org bot's idle desktop is shut down unless the bot has Keep Alive on.
func (suite *PostgresStoreTestSuite) TestPostgresStore_ListIdleDesktops_SkipsKeepAliveOrgBot() {
	ctx := context.Background()
	org := &types.Organization{ID: "org_keepalive_" + system.GenerateUUID(), Name: "keepalive-" + system.GenerateUUID(), Owner: "user_id"}
	_, err := suite.db.CreateOrganization(ctx, org)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _ = suite.db.DeleteOrganization(ctx, org.ID) })

	botID := "b-keepalive"
	suite.Require().NoError(suite.db.gdb.WithContext(ctx).Exec(
		`INSERT INTO org_bots (id, org_id, name, content, keep_alive, created_at, updated_at) VALUES (?, ?, 'Keep', '# Keep', false, now(), now())`,
		botID, org.ID).Error)

	session := types.Session{
		ID:             system.GenerateSessionID(),
		Owner:          "user_id",
		OrganizationID: org.ID,
		Created:        time.Now(),
		Updated:        time.Now().Add(-2 * time.Hour),
		Metadata: types.SessionMetadata{
			ExternalAgentStatus: "running",
			DevContainerID:      "container-orgbot-" + system.GenerateUUID(),
			OrgWorkerID:         botID,
		},
	}
	_, err = suite.db.CreateSession(ctx, session)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
	oldTime := time.Now().Add(-2 * time.Hour)
	_, err = suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, GenerationID: 1,
		UserID: "user_id", Created: oldTime, Updated: oldTime,
	})
	suite.Require().NoError(err)

	idle := func() bool {
		results, err := suite.db.ListIdleDesktops(ctx, time.Now().Add(-1*time.Hour))
		suite.Require().NoError(err)
		for _, s := range results {
			if s.ID == session.ID {
				return true
			}
		}
		return false
	}
	suite.True(idle(), "without Keep Alive the bot's idle desktop is shut down")

	suite.Require().NoError(suite.db.gdb.WithContext(ctx).Exec(
		`UPDATE org_bots SET keep_alive = true WHERE id = ? AND org_id = ?`, botID, org.ID).Error)
	suite.False(idle(), "Keep Alive exempts the bot's desktop")
}
