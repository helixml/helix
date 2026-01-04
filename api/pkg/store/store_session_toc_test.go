package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TestPostgresStore_SessionTOC tests the session table of contents functionality
func (suite *PostgresStoreTestSuite) TestPostgresStore_SessionTOC_ListInteractionsForTOC() {
	// Create a session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "Test TOC Session",
		Owner:   "user_toc_test",
		Created: time.Now(),
		Updated: time.Now(),
	}

	createdSession, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Create 5 interactions with prompts and responses
	for i := 0; i < 5; i++ {
		interaction := &types.Interaction{
			ID:              system.GenerateInteractionID(),
			SessionID:       createdSession.ID,
			GenerationID:    0,
			UserID:          "user_toc_test",
			PromptMessage:   fmt.Sprintf("Turn %d: User asks a question about topic %d", i+1, i+1),
			ResponseMessage: fmt.Sprintf("Turn %d: Assistant provides helpful answer", i+1),
			State:           types.InteractionStateComplete,
			Created:         time.Now().Add(time.Duration(i) * time.Minute),
			Updated:         time.Now().Add(time.Duration(i) * time.Minute),
		}
		err := suite.db.CreateInteractions(context.Background(), interaction)
		suite.NoError(err)
	}

	// List interactions - should be returned in chronological order
	interactions, total, err := suite.db.ListInteractions(context.Background(), &types.ListInteractionsQuery{
		SessionID:    createdSession.ID,
		GenerationID: 0,
		PerPage:      50,
	})

	suite.NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(interactions, 5)

	// Verify each turn has content
	for i, interaction := range interactions {
		suite.Contains(interaction.PromptMessage, fmt.Sprintf("Turn %d:", i+1))
		suite.Contains(interaction.ResponseMessage, fmt.Sprintf("Turn %d:", i+1))
	}
}

// TestPostgresStore_UpdateInteractionSummary tests saving summaries to interactions
func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateInteractionSummary() {
	// Create a session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "Summary Test Session",
		Owner:   "user_summary_test",
		Created: time.Now(),
		Updated: time.Now(),
	}

	createdSession, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Create an interaction without a summary
	interaction := &types.Interaction{
		ID:              system.GenerateInteractionID(),
		SessionID:       createdSession.ID,
		GenerationID:    0,
		UserID:          "user_summary_test",
		PromptMessage:   "Tell me about session summaries",
		ResponseMessage: "Session summaries help agents navigate conversation history...",
		State:           types.InteractionStateComplete,
		Created:         time.Now(),
		Updated:         time.Now(),
	}
	err = suite.db.CreateInteractions(context.Background(), interaction)
	suite.NoError(err)

	// Update the summary
	testSummary := "Discussed session summaries feature"
	err = suite.db.UpdateInteractionSummary(context.Background(), interaction.ID, testSummary)
	suite.NoError(err)

	// Retrieve and verify
	retrieved, err := suite.db.GetInteraction(context.Background(), interaction.ID)
	suite.NoError(err)
	suite.Equal(testSummary, retrieved.Summary)
	suite.NotNil(retrieved.SummaryUpdatedAt)
}

// TestPostgresStore_TitleHistory tests the title history tracking in session metadata
func (suite *PostgresStoreTestSuite) TestPostgresStore_TitleHistory() {
	// Create a session with initial metadata
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "Initial Title",
		Owner:   "user_title_history",
		Created: time.Now(),
		Updated: time.Now(),
		Metadata: types.SessionMetadata{
			TitleHistory: []*types.TitleHistoryEntry{},
		},
	}

	createdSession, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Create an interaction (which would trigger title generation)
	interaction := &types.Interaction{
		ID:              system.GenerateInteractionID(),
		SessionID:       createdSession.ID,
		GenerationID:    0,
		UserID:          "user_title_history",
		PromptMessage:   "Let's discuss MCP servers",
		ResponseMessage: "MCP servers provide tools to AI agents...",
		State:           types.InteractionStateComplete,
		Created:         time.Now(),
		Updated:         time.Now(),
	}
	err = suite.db.CreateInteractions(context.Background(), interaction)
	suite.NoError(err)

	// Simulate title update with history entry
	newMetadata := types.SessionMetadata{
		TitleHistory: []*types.TitleHistoryEntry{
			{
				Title:         "Discussing MCP servers",
				ChangedAt:     time.Now(),
				Turn:          1,
				InteractionID: interaction.ID,
			},
		},
	}

	err = suite.db.UpdateSessionMetadata(context.Background(), createdSession.ID, newMetadata)
	suite.NoError(err)

	// Update session name
	err = suite.db.UpdateSessionName(context.Background(), createdSession.ID, "Discussing MCP servers")
	suite.NoError(err)

	// Retrieve and verify
	retrieved, err := suite.db.GetSession(context.Background(), createdSession.ID)
	suite.NoError(err)
	suite.Equal("Discussing MCP servers", retrieved.Name)
	suite.Len(retrieved.Metadata.TitleHistory, 1)
	suite.Equal("Discussing MCP servers", retrieved.Metadata.TitleHistory[0].Title)
	suite.Equal(1, retrieved.Metadata.TitleHistory[0].Turn)
	suite.Equal(interaction.ID, retrieved.Metadata.TitleHistory[0].InteractionID)
}

// TestPostgresStore_TitleHistoryMultipleEntries tests multiple title history entries
func (suite *PostgresStoreTestSuite) TestPostgresStore_TitleHistoryMultipleEntries() {
	// Create a session
	session := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "First Topic",
		Owner:   "user_multi_title",
		Created: time.Now(),
		Updated: time.Now(),
	}

	createdSession, err := suite.db.CreateSession(context.Background(), session)
	suite.NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), session.ID)
	})

	// Simulate multiple topic changes (newest first as per design)
	titleHistory := []*types.TitleHistoryEntry{
		{
			Title:         "Third Topic - MCP Integration",
			ChangedAt:     time.Now(),
			Turn:          6,
			InteractionID: "int_003",
		},
		{
			Title:         "Second Topic - WebSocket Updates",
			ChangedAt:     time.Now().Add(-10 * time.Minute),
			Turn:          3,
			InteractionID: "int_002",
		},
		{
			Title:         "First Topic",
			ChangedAt:     time.Now().Add(-20 * time.Minute),
			Turn:          1,
			InteractionID: "int_001",
		},
	}

	newMetadata := types.SessionMetadata{
		TitleHistory: titleHistory,
	}

	err = suite.db.UpdateSessionMetadata(context.Background(), createdSession.ID, newMetadata)
	suite.NoError(err)

	// Retrieve and verify order is preserved (newest first)
	retrieved, err := suite.db.GetSession(context.Background(), createdSession.ID)
	suite.NoError(err)
	suite.Len(retrieved.Metadata.TitleHistory, 3)
	suite.Equal("Third Topic - MCP Integration", retrieved.Metadata.TitleHistory[0].Title)
	suite.Equal("Second Topic - WebSocket Updates", retrieved.Metadata.TitleHistory[1].Title)
	suite.Equal("First Topic", retrieved.Metadata.TitleHistory[2].Title)
}
