package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
	"gorm.io/datatypes"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_InteractionQuestionLifecycle() {
	ctx := context.Background()
	userID := "user-question-test"
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: userID, Created: time.Now(), Updated: time.Now(),
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	interaction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: userID,
		GenerationID: 1, State: types.InteractionStateWaiting,
	})
	suite.Require().NoError(err)
	question := &types.PendingQuestion{
		RequestID: "question-1", ThreadID: "thread-1", TurnRequestID: "turn-1", Source: "elicitation",
		Questions: []types.UserQuestion{{ID: "choice", Question: "Choose one"}},
	}

	updated, changed, err := suite.db.SetInteractionPendingQuestion(ctx, interaction.ID, 1, question)
	suite.Require().NoError(err)
	suite.True(changed)
	suite.Require().NotNil(updated.PendingQuestion)

	interaction.ResponseMessage = "still working"
	_, err = suite.db.UpdateInteraction(ctx, interaction)
	suite.Require().NoError(err)
	updated, err = suite.db.GetInteraction(ctx, interaction.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(updated.PendingQuestion)

	updated, changed, err = suite.db.ResolveInteractionPendingQuestion(
		ctx, interaction.ID, 1, question.RequestID, "answered", map[string]string{"choice": "A"},
	)
	suite.Require().NoError(err)
	suite.True(changed)
	suite.Nil(updated.PendingQuestion)
	suite.Require().Len(updated.QuestionHistory, 1)
	suite.Equal("A", updated.QuestionHistory[0].Answers["choice"])

	updated, changed, err = suite.db.SetInteractionPendingQuestion(ctx, interaction.ID, 1, question)
	suite.Require().NoError(err)
	suite.False(changed)
	suite.Nil(updated.PendingQuestion)
	suite.Len(updated.QuestionHistory, 1)

	_, _, err = suite.db.SetInteractionPendingQuestion(ctx, "missing-interaction", 1, question)
	suite.ErrorIs(err, ErrNotFound)
	_, _, err = suite.db.ResolveInteractionPendingQuestion(ctx, "missing-interaction", 1, question.RequestID, "cancelled", nil)
	suite.ErrorIs(err, ErrNotFound)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ReapSettlesPendingQuestion() {
	ctx := context.Background()
	userID := "user-reap-question-test"
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: userID, Created: time.Now(), Updated: time.Now(),
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	interaction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: userID,
		GenerationID: 1, State: types.InteractionStateWaiting,
	})
	suite.Require().NoError(err)
	_, changed, err := suite.db.SetInteractionPendingQuestion(ctx, interaction.ID, interaction.GenerationID, &types.PendingQuestion{
		RequestID: "question-reap", ThreadID: "thread-reap", TurnRequestID: "turn-reap", Source: "elicitation",
		Questions: []types.UserQuestion{{ID: "choice", Question: "Choose one"}},
	})
	suite.Require().NoError(err)
	suite.True(changed)

	reaped, err := suite.db.ReapWaitingInteractions(ctx, session.ID, types.InteractionStateInterrupted, "test reap")
	suite.Require().NoError(err)
	suite.Require().Len(reaped, 1)
	suite.Nil(reaped[0].PendingQuestion)
	suite.Require().Len(reaped[0].QuestionHistory, 1)
	suite.Equal("cancelled", reaped[0].QuestionHistory[0].Outcome)

	updated, err := suite.db.GetInteraction(ctx, interaction.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateInterrupted, updated.State)
	suite.Nil(updated.PendingQuestion)
	suite.Require().Len(updated.QuestionHistory, 1)
	suite.Equal("question-reap", updated.QuestionHistory[0].RequestID)
}

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

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetLatestInteractionsForSessionsUsesUpdated() {
	ctx := context.Background()
	userID := "user-latest-interaction-test"
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: userID, Created: time.Now(), Updated: time.Now(),
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	base := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	waiting := &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: userID,
		GenerationID: 1, State: types.InteractionStateWaiting,
		Created: base.Add(time.Minute), Updated: base.Add(3 * time.Minute), PromptMessage: "user prompt",
	}
	copiedHire := &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: userID,
		GenerationID: 1, State: types.InteractionStateComplete, Trigger: "org_hire",
		Created: base.Add(2 * time.Minute), Updated: base, PromptMessage: "copied hire prompt",
	}
	_, err = suite.db.CreateInteraction(ctx, waiting)
	suite.Require().NoError(err)
	_, err = suite.db.CreateInteraction(ctx, copiedHire)
	suite.Require().NoError(err)

	latest, err := suite.db.GetLatestInteractionsForSessions(ctx, []string{session.ID})
	suite.Require().NoError(err)
	suite.Require().Contains(latest, session.ID)
	suite.Equal(types.InteractionStateWaiting, latest[session.ID].State)
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
		State:         types.InteractionStateWaiting,
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

	transitioned, err := suite.db.MarkInteractionCompleteIfWaiting(context.Background(), interaction.ID, interaction.GenerationID)
	suite.NoError(err)
	suite.True(transitioned)
	transitioned, err = suite.db.MarkInteractionErrorIfWaiting(context.Background(), interaction.ID, interaction.GenerationID, "must not overwrite")
	suite.NoError(err)
	suite.False(transitioned)
	gotInteraction, err = suite.db.GetInteraction(context.Background(), interaction.ID)
	suite.NoError(err)
	suite.Equal(types.InteractionStateComplete, gotInteraction.State)
	suite.Equal("foobar2", gotInteraction.ResponseMessage)

	waiting := &types.Interaction{
		ID:           system.GenerateInteractionID(),
		SessionID:    session.ID,
		GenerationID: 1,
		UserID:       userID,
		State:        types.InteractionStateWaiting,
	}
	_, err = suite.db.CreateInteraction(context.Background(), waiting)
	suite.NoError(err)
	transitioned, err = suite.db.MarkInteractionErrorIfWaiting(context.Background(), waiting.ID, waiting.GenerationID, "agent unresponsive")
	suite.NoError(err)
	suite.True(transitioned)
	gotInteraction, err = suite.db.GetInteraction(context.Background(), waiting.ID)
	suite.NoError(err)
	suite.Equal(types.InteractionStateError, gotInteraction.State)
	suite.Equal("agent unresponsive", gotInteraction.Error)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListInteractionsFiltersStateBeforeLimit() {
	ctx := context.Background()
	userID := "user-state-filter-test"
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: userID, Created: time.Now(), Updated: time.Now(),
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })

	_, err = suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: userID,
		GenerationID: 1, State: types.InteractionStateComplete,
	})
	suite.Require().NoError(err)
	waiting, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: userID,
		GenerationID: 1, State: types.InteractionStateWaiting,
	})
	suite.Require().NoError(err)

	interactions, total, err := suite.db.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: 1,
		PerPage:      1,
		State:        types.InteractionStateWaiting,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(interactions, 1)
	suite.Equal(waiting.ID, interactions[0].ID)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListStuckWaitingInteractionsIncludesStoppedPartialTurns() {
	ctx := context.Background()
	now := time.Now()
	owner := "user-stuck-partial-test"
	createSession := func(agentType, status string) *types.Session {
		session, err := suite.db.CreateSession(ctx, types.Session{
			ID: system.GenerateSessionID(), Owner: owner, Created: now, Updated: now,
			Metadata: types.SessionMetadata{AgentType: agentType, ExternalAgentStatus: status},
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
		return session
	}
	stopped := createSession("zed_external", "stopped")
	running := createSession("zed_external", "running")
	regular := createSession("helix", "stopped")
	archived := createSession("zed_external", "stopped")
	archived.Archived = true
	_, err := suite.db.UpdateSession(ctx, *archived)
	suite.Require().NoError(err)
	deleted := createSession("zed_external", "stopped")
	createInteraction := func(sessionID string, state types.InteractionState, updated time.Time, response string, entries datatypes.JSON) *types.Interaction {
		interaction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
			ID: system.GenerateInteractionID(), SessionID: sessionID, UserID: owner,
			GenerationID: 1, State: state, Created: updated, Updated: updated,
			ResponseMessage: response, ResponseEntries: entries,
		})
		suite.Require().NoError(err)
		return interaction
	}

	createInteraction(deleted.ID, types.InteractionStateWaiting, now.Add(-22*time.Minute), "deleted partial", nil)
	_, err = suite.db.DeleteSession(ctx, deleted.ID)
	suite.Require().NoError(err)
	createInteraction(archived.ID, types.InteractionStateWaiting, now.Add(-21*time.Minute), "archived partial", nil)
	createInteraction(running.ID, types.InteractionStateWaiting, now.Add(-20*time.Minute), "healthy partial", nil)
	createInteraction(stopped.ID, types.InteractionStateComplete, now.Add(-19*time.Minute), "done", nil)
	createInteraction(regular.ID, types.InteractionStateWaiting, now.Add(-18*time.Minute), "not external", nil)
	partialEntries := createInteraction(stopped.ID, types.InteractionStateWaiting, now.Add(-10*time.Minute), "", datatypes.JSON(`[{"type":"tool_call"}]`))
	partialText := createInteraction(stopped.ID, types.InteractionStateWaiting, now.Add(-9*time.Minute), "partial output", nil)
	createInteraction(stopped.ID, types.InteractionStateWaiting, now.Add(-time.Minute), "recent output", nil)

	stuck, err := suite.db.ListStuckWaitingInteractions(ctx, now.Add(-3*time.Minute), 2)
	suite.Require().NoError(err)
	suite.Require().Len(stuck, 2)
	suite.Equal(partialEntries.ID, stuck[0].ID)
	suite.Equal(partialText.ID, stuck[1].ID)

	transitioned, err := suite.db.MarkStaleInteractionErrorIfWaiting(ctx, partialText.ID, partialText.GenerationID, stuck[1].Updated, "agent stopped")
	suite.Require().NoError(err)
	suite.True(transitioned)
	settled, err := suite.db.GetInteraction(ctx, partialText.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateError, settled.State)
	suite.Equal("partial output", settled.ResponseMessage)

	observedUpdated := stuck[0].Updated
	suite.Require().NoError(suite.db.UpdateInteractionStreamingFields(
		ctx, partialEntries.ID, partialEntries.GenerationID, "resumed output", datatypes.JSON(`[{"type":"text","content":"resumed"}]`), 0, "msg-2",
	))
	transitioned, err = suite.db.MarkStaleInteractionErrorIfWaiting(ctx, partialEntries.ID, partialEntries.GenerationID, observedUpdated, "must not overwrite resumed stream")
	suite.Require().NoError(err)
	suite.False(transitioned)
	resumed, err := suite.db.GetInteraction(ctx, partialEntries.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateWaiting, resumed.State)
	suite.Equal("resumed output", resumed.ResponseMessage)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ClearSessionInteractions() {
	userID := "user-clear-test"
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

	// Clearing an already-empty session is a no-op and returns success.
	err = suite.db.ClearSessionInteractions(ctx, session.ID)
	suite.NoError(err)

	// Seed N interactions.
	const n = 3
	for i := 0; i < n; i++ {
		_, err = suite.db.CreateInteraction(ctx, &types.Interaction{
			ID:            system.GenerateInteractionID(),
			SessionID:     session.ID,
			GenerationID:  1,
			UserID:        userID,
			Created:       time.Now(),
			Updated:       time.Now(),
			PromptMessage: "msg",
		})
		suite.NoError(err)
	}

	_, total, err := suite.db.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: 1,
	})
	suite.NoError(err)
	suite.Equal(n, int(total), "should have seeded N interactions")

	// Clear them.
	err = suite.db.ClearSessionInteractions(ctx, session.ID)
	suite.NoError(err)

	// Zero interactions remain.
	_, total, err = suite.db.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: 1,
	})
	suite.NoError(err)
	suite.Equal(0, int(total), "all interactions should be cleared")

	// Session row is preserved.
	gotSession, err := suite.db.GetSession(ctx, session.ID)
	suite.NoError(err)
	suite.Equal(session.ID, gotSession.ID)

	// Idempotent: clearing again succeeds.
	err = suite.db.ClearSessionInteractions(ctx, session.ID)
	suite.NoError(err)
}

func (suite *PostgresStoreTestSuite) TestExternalAgentInteractionLifecycleSurvivesAPIRestart() {
	ctx := context.Background()
	owner := "external-lifecycle-test"
	externalSession, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: owner, Created: time.Now(), Updated: time.Now(),
		Metadata: types.SessionMetadata{AgentType: "zed_external"},
	})
	suite.Require().NoError(err)
	internalSession, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: owner, Created: time.Now(), Updated: time.Now(),
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(ctx, externalSession.ID)
		_, _ = suite.db.DeleteSession(ctx, internalSession.ID)
	})

	external, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: externalSession.ID, UserID: owner,
		GenerationID: 1, State: types.InteractionStateWaiting,
	})
	suite.Require().NoError(err)
	internal, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: internalSession.ID, UserID: owner,
		GenerationID: 1, State: types.InteractionStateWaiting,
	})
	suite.Require().NoError(err)

	bound, err := suite.db.BindInteractionExternalAgentRequest(ctx, external.ID, external.GenerationID, "req-durable")
	suite.NoError(err)
	suite.True(bound)
	dispatched, err := suite.db.MarkInteractionExternalAgentDispatched(ctx, external.ID, external.GenerationID, "req-durable")
	suite.NoError(err)
	suite.True(dispatched)

	byRequest, err := suite.db.GetInteractionByExternalAgentRequestID(ctx, "req-durable")
	suite.NoError(err)
	suite.Equal(external.ID, byRequest.ID)
	suite.NotNil(byRequest.ExternalAgentDispatchedAt)

	suite.NoError(suite.db.ResetRunningInteractions(ctx))
	externalAfterRestart, err := suite.db.GetInteraction(ctx, external.ID)
	suite.NoError(err)
	suite.Equal(types.InteractionStateWaiting, externalAfterRestart.State,
		"an API restart must not terminate a runtime that lives in Zed")
	internalAfterRestart, err := suite.db.GetInteraction(ctx, internal.ID)
	suite.NoError(err)
	suite.Equal(types.InteractionStateError, internalAfterRestart.State)

	requested, err := suite.db.RequestInteractionCancellationIfWaiting(ctx, external.ID, external.GenerationID)
	suite.NoError(err)
	suite.True(requested)
	interrupted, err := suite.db.MarkInteractionInterruptedIfWaiting(ctx, external.ID, external.GenerationID)
	suite.NoError(err)
	suite.True(interrupted)
	final, err := suite.db.GetInteraction(ctx, external.ID)
	suite.NoError(err)
	suite.Equal(types.InteractionStateInterrupted, final.State)
	suite.NotNil(final.ExternalAgentCancelRequestedAt)
}
