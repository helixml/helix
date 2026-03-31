package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetInteractionsSummary() {
	userID := "user-summary-test"
	ctx := context.Background()

	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   userID,
		Created: time.Now(),
		Updated: time.Now(),
	}
	createdSession, err := suite.db.CreateSession(ctx, session)
	suite.NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(ctx, createdSession.ID)
	})

	// Empty session — should return 0 count and zero time
	count, maxUpdated, err := suite.db.GetInteractionsSummary(ctx, session.ID, 0)
	suite.NoError(err)
	suite.Equal(int64(0), count)
	suite.True(maxUpdated.IsZero(), "maxUpdated should be zero for empty session")

	// Add first interaction
	t1 := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	i1 := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		SessionID:     session.ID,
		GenerationID:  1,
		UserID:        userID,
		Created:       t1,
		Updated:       t1,
		PromptMessage: "first message",
	}
	_, err = suite.db.CreateInteraction(ctx, i1)
	suite.NoError(err)

	count, maxUpdated, err = suite.db.GetInteractionsSummary(ctx, session.ID, 1)
	suite.NoError(err)
	suite.Equal(int64(1), count)
	suite.Equal(t1.UTC(), maxUpdated.UTC())

	// Add second interaction with a later timestamp
	t2 := time.Date(2026, 3, 31, 11, 0, 0, 0, time.UTC)
	i2 := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		SessionID:     session.ID,
		GenerationID:  1,
		UserID:        userID,
		Created:       t2,
		Updated:       t2,
		PromptMessage: "second message",
	}
	_, err = suite.db.CreateInteraction(ctx, i2)
	suite.NoError(err)

	count, maxUpdated, err = suite.db.GetInteractionsSummary(ctx, session.ID, 1)
	suite.NoError(err)
	suite.Equal(int64(2), count)
	suite.Equal(t2.UTC(), maxUpdated.UTC())

	// Update the first interaction — maxUpdated should change
	t3 := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	i1.Updated = t3
	i1.ResponseMessage = "response"
	_, err = suite.db.UpdateInteraction(ctx, i1)
	suite.NoError(err)

	count, maxUpdated, err = suite.db.GetInteractionsSummary(ctx, session.ID, 1)
	suite.NoError(err)
	suite.Equal(int64(2), count, "count should not change on update")
	suite.Equal(t3.UTC(), maxUpdated.UTC(), "maxUpdated should reflect the updated interaction")

	// Different generation ID — should only see interactions for that generation
	t4 := time.Date(2026, 3, 31, 13, 0, 0, 0, time.UTC)
	i3 := &types.Interaction{
		ID:            system.GenerateInteractionID(),
		SessionID:     session.ID,
		GenerationID:  2,
		UserID:        userID,
		Created:       t4,
		Updated:       t4,
		PromptMessage: "regenerated message",
	}
	_, err = suite.db.CreateInteraction(ctx, i3)
	suite.NoError(err)

	count, maxUpdated, err = suite.db.GetInteractionsSummary(ctx, session.ID, 2)
	suite.NoError(err)
	suite.Equal(int64(1), count, "generation 2 should have only 1 interaction")
	suite.Equal(t4.UTC(), maxUpdated.UTC())

	// Generation 1 should still have 2
	count, _, err = suite.db.GetInteractionsSummary(ctx, session.ID, 1)
	suite.NoError(err)
	suite.Equal(int64(2), count, "generation 1 should still have 2 interactions")

	// Generation 0 (no filter) should see all 3
	count, maxUpdated, err = suite.db.GetInteractionsSummary(ctx, session.ID, 0)
	suite.NoError(err)
	suite.Equal(int64(3), count, "generation 0 should see all interactions")
	suite.Equal(t4.UTC(), maxUpdated.UTC(), "maxUpdated should be the latest across all generations")

	// Stability: calling GetInteractionsSummary repeatedly without changes must
	// return identical values. This is what the ETag is derived from — any jitter
	// (e.g., timestamp precision loss) would cause spurious cache misses.
	for i := 0; i < 5; i++ {
		c, mu, err := suite.db.GetInteractionsSummary(ctx, session.ID, 1)
		suite.NoError(err)
		suite.Equal(int64(2), c, "count must be stable across calls (iteration %d)", i)
		suite.Equal(t3.UTC(), mu.UTC(), "maxUpdated must be stable across calls (iteration %d)", i)
	}
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_Interactions() {
	userID := "user-id-1"
	// Create a sample session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   userID,
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Call the CreateSession method
	createdSession, err := suite.db.CreateSession(context.Background(), session)

	// Assert that no error occurred
	suite.NoError(err)

	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), createdSession.ID)
	})

	// Create interaction for the session
	interaction := types.Interaction{
		ID:            system.GenerateInteractionID(),
		SessionID:     session.ID,
		GenerationID:  1,
		UserID:        userID,
		PromptMessage: "hello",
		PromptMessageContent: types.MessageContent{
			ContentType: types.MessageContentTypeText,
			Parts: []any{
				"hello",
			},
		},
		ToolCalls: []openai.ToolCall{
			{
				ID: "1",
			},
		},
		ResponseMessage: "",
		Usage: types.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
			DurationMs:       1000,
		},
	}

	// Call the CreateInteraction method
	createdInteraction, err := suite.db.CreateInteraction(context.Background(), &interaction)
	// Assert that no error occurred
	suite.NoError(err)
	// Assert that the created interaction matches the original interaction
	suite.Equal(interaction, *createdInteraction)

	// Update interaction
	interaction.ResponseMessage = "foobar2"
	_, err = suite.db.UpdateInteraction(context.Background(), &interaction)
	suite.NoError(err)

	// Get it
	gotInteraction, err := suite.db.GetInteraction(context.Background(), createdInteraction.ID)
	suite.NoError(err)
	suite.Equal("foobar2", gotInteraction.ResponseMessage)

	// List
	interactions, total, err := suite.db.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: 1,
	})
	suite.NoError(err)
	suite.Equal(1, int(total))
	suite.Equal(createdInteraction.ID, interactions[0].ID)
}
