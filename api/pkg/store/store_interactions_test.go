package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
)

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
