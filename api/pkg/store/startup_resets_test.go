package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// A new blue/green colour serves requests before it becomes leader and runs
// the startup resets. The resets must fail work a gone process left behind
// (started before the cutoff) and leave the new colour's own work alone.
func (suite *PostgresStoreTestSuite) TestStartupResetsSpareWorkAfterTheCutoff() {
	ctx := context.Background()
	owner := "user-startup-resets"

	newWaiting := func(created time.Time) *types.Interaction {
		session, err := suite.db.CreateSession(ctx, types.Session{
			ID: system.GenerateSessionID(), Owner: owner, Created: created, Updated: created,
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
		interaction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
			ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: owner,
			GenerationID: 1, State: types.InteractionStateWaiting, Created: created, Updated: created,
		})
		suite.Require().NoError(err)
		return interaction
	}
	cutoff := time.Now()
	abandoned := newWaiting(cutoff.Add(-time.Minute))
	own := newWaiting(cutoff.Add(time.Second))

	suite.Require().NoError(suite.db.ResetRunningInteractions(ctx, cutoff))
	got, err := suite.db.GetInteraction(ctx, abandoned.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateError, got.State, "an interaction from before the cutoff is abandoned")
	got, err = suite.db.GetInteraction(ctx, own.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateWaiting, got.State, "an interaction from after the cutoff is still being served")

	// Sessions: MarkSessionStartingIfIdle stamps updated, which is what the
	// cutoff compares.
	newStarting := func() string {
		session, err := suite.db.CreateSession(ctx, types.Session{
			ID: system.GenerateSessionID(), Owner: owner,
			Created: time.Now().Add(-time.Hour), Updated: time.Now().Add(-time.Hour),
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
		marked, err := suite.db.MarkSessionStartingIfIdle(ctx, session.ID)
		suite.Require().NoError(err)
		suite.Require().True(marked)
		return session.ID
	}
	stale := newStarting()
	time.Sleep(50 * time.Millisecond)
	sessionCutoff := time.Now()
	time.Sleep(50 * time.Millisecond)
	starting := newStarting()

	_, err = suite.db.ClearStaleStartingSessions(ctx, sessionCutoff)
	suite.Require().NoError(err)
	staleSession, err := suite.db.GetSession(ctx, stale)
	suite.Require().NoError(err)
	suite.Equal("stopped", staleSession.Metadata.ExternalAgentStatus)
	startingSession, err := suite.db.GetSession(ctx, starting)
	suite.Require().NoError(err)
	suite.Equal("starting", startingSession.Metadata.ExternalAgentStatus,
		"a start begun after the cutoff belongs to this process")
}
