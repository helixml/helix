package store

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestCreatePromptHistoryEntryIfAbsentConcurrent() {
	ctx := context.Background()
	id := "phe_activation_" + system.GenerateUUID()
	sessionID := "ses_activation_" + system.GenerateUUID()
	entries := []*types.PromptHistoryEntry{
		{ID: id, UserID: "user_activation", SessionID: sessionID, Content: "first delivery", Status: "pending"},
		{ID: id, UserID: "user_activation", SessionID: sessionID, Content: "redelivery", Status: "failed"},
	}
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM prompt_history_entries WHERE id = ?", id)
	})

	start := make(chan struct{})
	created := make([]bool, len(entries))
	errs := make([]error, len(entries))
	var wg sync.WaitGroup
	for i := range entries {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			created[i], errs[i] = suite.db.CreatePromptHistoryEntryIfAbsent(ctx, entries[i])
		}(i)
	}
	close(start)
	wg.Wait()

	winner := -1
	for i := range entries {
		suite.Require().NoError(errs[i])
		if created[i] {
			suite.Equal(-1, winner, "only one concurrent insert may create the row")
			winner = i
		}
	}
	suite.Require().NotEqual(-1, winner)

	stored, err := suite.db.GetPromptHistoryEntry(ctx, id)
	suite.Require().NoError(err)
	suite.Require().NotNil(stored)
	suite.Equal(entries[winner].Content, stored.Content)
	suite.Equal(entries[winner].Status, stored.Status)
}

func (suite *PostgresStoreTestSuite) TestCreatePromptHistoryEntryIfAbsentDistinctIDs() {
	ctx := context.Background()
	sessionID := "ses_activation_" + system.GenerateUUID()
	ids := []string{"phe_activation_" + system.GenerateUUID(), "phe_activation_" + system.GenerateUUID()}
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM prompt_history_entries WHERE id IN ?", ids)
	})

	for _, id := range ids {
		created, err := suite.db.CreatePromptHistoryEntryIfAbsent(ctx, &types.PromptHistoryEntry{
			ID: id, UserID: "user_activation", SessionID: sessionID, Content: "same activation", Status: "pending",
		})
		suite.Require().NoError(err)
		suite.True(created)
	}

	var count int64
	suite.Require().NoError(suite.db.gdb.Model(&types.PromptHistoryEntry{}).Where("id IN ?", ids).Count(&count).Error)
	suite.Equal(int64(2), count)
}

func (suite *PostgresStoreTestSuite) TestGetLatestInteractionByPromptID() {
	ctx := context.Background()
	promptID := "phe_activation_" + system.GenerateUUID()
	base := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: "user_activation", Created: base, Updated: base,
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
	sessionID := session.ID
	old := &types.Interaction{
		ID: "int_activation_old_" + system.GenerateUUID(), GenerationID: 1, PromptID: promptID,
		SessionID: sessionID, UserID: "user_activation", Created: base, Updated: base, State: types.InteractionStateError,
	}
	current := &types.Interaction{
		ID: "int_activation_current_" + system.GenerateUUID(), GenerationID: 1, PromptID: promptID,
		SessionID: sessionID, UserID: "user_activation", Created: base.Add(time.Minute), Updated: base.Add(time.Minute), State: types.InteractionStateWaiting,
	}
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM interactions WHERE prompt_id = ?", promptID)
	})
	_, err = suite.db.CreateInteraction(ctx, old)
	suite.Require().NoError(err)
	_, err = suite.db.CreateInteraction(ctx, current)
	suite.Require().NoError(err)

	latest, err := suite.db.GetLatestInteractionByPromptID(ctx, promptID)
	suite.Require().NoError(err)
	suite.Require().NotNil(latest)
	suite.Equal(current.ID, latest.ID)
	suite.Equal(types.InteractionStateWaiting, latest.State)

	current.State = types.InteractionStateComplete
	_, err = suite.db.UpdateInteraction(ctx, current)
	suite.Require().NoError(err)
	latest, err = suite.db.GetLatestInteractionByPromptID(ctx, promptID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateComplete, latest.State)

	tiedPromptID := "phe_activation_tied_" + system.GenerateUUID()
	tiedIDs := []string{"int_activation_tie_a_" + system.GenerateUUID(), "int_activation_tie_b_" + system.GenerateUUID()}
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM interactions WHERE prompt_id = ?", tiedPromptID)
	})
	for _, id := range tiedIDs {
		_, err = suite.db.CreateInteraction(ctx, &types.Interaction{
			ID: id, GenerationID: 1, PromptID: tiedPromptID, SessionID: sessionID,
			UserID: "user_activation", Created: base, Updated: base, State: types.InteractionStateWaiting,
		})
		suite.Require().NoError(err)
	}
	latest, err = suite.db.GetLatestInteractionByPromptID(ctx, tiedPromptID)
	suite.Require().NoError(err)
	suite.Equal(tiedIDs[1], latest.ID)
}
