package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestRecoverProviderPromptClaimsExactFailedInteractionOnce() {
	ctx := context.Background()
	now := time.Now()
	promptID := "phe_provider_recovery_" + system.GenerateUUID()
	interactionID := "int_provider_recovery_" + system.GenerateUUID()
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: "user_provider_recovery", Created: now, Updated: now,
		Metadata: types.SessionMetadata{Recovery: &types.ProviderRecovery{
			Provider: "pe-1", PromptID: promptID, InteractionID: interactionID, NextCheckAt: now.Add(-time.Second),
		}},
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
	suite.Require().NoError(suite.db.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: promptID, UserID: session.Owner, SessionID: session.ID, Content: "retry me", Status: "sent",
	}))
	interaction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: interactionID, SessionID: session.ID, UserID: session.Owner, PromptID: promptID,
		Created: now, Updated: now, State: types.InteractionStateError,
	})
	suite.Require().NoError(err)

	claimed, err := suite.db.RecoverProviderPrompt(ctx, session.ID, promptID, now, now.Add(time.Minute), 3)
	suite.Require().NoError(err)
	suite.True(claimed)
	updatedSession, err := suite.db.GetSession(ctx, session.ID)
	suite.Require().NoError(err)
	suite.Equal(1, updatedSession.Metadata.Recovery.Attempts)
	prompt, err := suite.db.GetPromptHistoryEntry(ctx, promptID)
	suite.Require().NoError(err)
	suite.Equal("pending", prompt.Status)

	claimed, err = suite.db.RecoverProviderPrompt(ctx, session.ID, promptID, now, now.Add(time.Minute), 3)
	suite.Require().NoError(err)
	suite.False(claimed)

	interaction.State = types.InteractionStateComplete
	_, err = suite.db.UpdateInteraction(ctx, interaction)
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.MarkPromptAsSent(ctx, promptID))
	updatedSession.Metadata.Recovery.NextCheckAt = now.Add(-time.Second)
	suite.Require().NoError(suite.db.SetSessionProviderRecovery(ctx, session.ID, updatedSession.Metadata.Recovery))
	claimed, err = suite.db.RecoverProviderPrompt(ctx, session.ID, promptID, now, now.Add(time.Minute), 3)
	suite.Require().NoError(err)
	suite.False(claimed)
	prompt, err = suite.db.GetPromptHistoryEntry(ctx, promptID)
	suite.Require().NoError(err)
	suite.Equal("sent", prompt.Status)

	interaction.State = types.InteractionStateError
	_, err = suite.db.UpdateInteraction(ctx, interaction)
	suite.Require().NoError(err)
	updatedSession.Metadata.Recovery.Attempts = 3
	updatedSession.Metadata.Recovery.NextCheckAt = now.Add(-time.Second)
	suite.Require().NoError(suite.db.SetSessionProviderRecovery(ctx, session.ID, updatedSession.Metadata.Recovery))
	claimed, err = suite.db.RecoverProviderPrompt(ctx, session.ID, promptID, now, now.Add(time.Minute), 3)
	suite.Require().NoError(err)
	suite.False(claimed)
	prompt, err = suite.db.GetPromptHistoryEntry(ctx, promptID)
	suite.Require().NoError(err)
	suite.Equal("sent", prompt.Status)
}

func (suite *PostgresStoreTestSuite) TestGetLatestActivationPromptHistoryEntryIncludesCancelled() {
	ctx := context.Background()
	now := time.Now()
	sessionID := system.GenerateSessionID()
	deletedAt := now
	entries := []*types.PromptHistoryEntry{
		{ID: "prompt_activation_old_" + system.GenerateUUID(), UserID: "user", SessionID: sessionID, Content: "old", Status: "sent", CreatedAt: now.Add(-time.Minute)},
		{ID: "prompt_activation_cancelled_" + system.GenerateUUID(), UserID: "user", SessionID: sessionID, Content: "cancelled", Status: "sent", CreatedAt: now, DeletedAt: &deletedAt},
		{ID: "promptXactivation_lookalike_" + system.GenerateUUID(), UserID: "user", SessionID: sessionID, Content: "lookalike", Status: "sent", CreatedAt: now.Add(time.Minute)},
	}
	for _, entry := range entries {
		suite.Require().NoError(suite.db.CreatePromptHistoryEntry(ctx, entry))
		suite.T().Cleanup(func() {
			suite.db.gdb.WithContext(ctx).Where("id = ?", entry.ID).Delete(&types.PromptHistoryEntry{})
		})
	}

	latest, err := suite.db.GetLatestActivationPromptHistoryEntry(ctx, sessionID)
	suite.Require().NoError(err)
	suite.Require().NotNil(latest)
	suite.Equal(entries[1].ID, latest.ID)
	suite.Require().NotNil(latest.DeletedAt)
}

func (suite *PostgresStoreTestSuite) TestRecoverFailedPromptOnlyClaimsLatestUncancelledFailure() {
	ctx := context.Background()
	now := time.Now()
	session, err := suite.db.CreateSession(ctx, types.Session{
		ID: system.GenerateSessionID(), Owner: "user_config_recovery", Created: now, Updated: now,
		Metadata: types.SessionMetadata{Recovery: &types.ProviderRecovery{PromptID: "stale"}},
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, session.ID) })
	cleared, err := suite.db.ClearSessionProviderRecovery(ctx, session.ID, "newer", "")
	suite.Require().NoError(err)
	suite.False(cleared)
	unchangedSession, err := suite.db.GetSession(ctx, session.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(unchangedSession.Metadata.Recovery)
	suite.Equal("stale", unchangedSession.Metadata.Recovery.PromptID)
	cleared, err = suite.db.ClearSessionProviderRecovery(ctx, session.ID, "stale", "")
	suite.Require().NoError(err)
	suite.True(cleared)

	oldPromptID := "prompt_activation_old_" + system.GenerateUUID()
	suite.Require().NoError(suite.db.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: oldPromptID, UserID: session.Owner, SessionID: session.ID, Content: "old", Status: "sent",
	}))
	oldInteraction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: session.Owner, PromptID: oldPromptID,
		Created: now, Updated: now, State: types.InteractionStateError,
	})
	suite.Require().NoError(err)
	_, err = suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: session.Owner, PromptID: oldPromptID,
		Created: now.Add(time.Second), Updated: now.Add(time.Second), State: types.InteractionStateComplete,
	})
	suite.Require().NoError(err)
	claimed, err := suite.db.RecoverFailedPrompt(ctx, session.ID, oldPromptID, oldInteraction.ID)
	suite.Require().NoError(err)
	suite.False(claimed)

	cancelledPromptID := "prompt_activation_cancelled_" + system.GenerateUUID()
	suite.Require().NoError(suite.db.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: cancelledPromptID, UserID: session.Owner, SessionID: session.ID, Content: "cancelled", Status: "sent",
	}))
	cancelledInteraction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: session.Owner, PromptID: cancelledPromptID,
		Created: now, Updated: now, State: types.InteractionStateError,
	})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.DeletePromptHistoryEntry(ctx, cancelledPromptID))
	claimed, err = suite.db.RecoverFailedPrompt(ctx, session.ID, cancelledPromptID, cancelledInteraction.ID)
	suite.Require().NoError(err)
	suite.False(claimed)

	eligiblePromptID := "prompt_activation_eligible_" + system.GenerateUUID()
	suite.Require().NoError(suite.db.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: eligiblePromptID, UserID: session.Owner, SessionID: session.ID, Content: "eligible", Status: "sent",
	}))
	eligibleInteraction, err := suite.db.CreateInteraction(ctx, &types.Interaction{
		ID: system.GenerateInteractionID(), SessionID: session.ID, UserID: session.Owner, PromptID: eligiblePromptID,
		Created: now, Updated: now, State: types.InteractionStateError,
	})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.SetSessionProviderRecovery(ctx, session.ID, &types.ProviderRecovery{
		PromptID: eligiblePromptID, InteractionID: eligibleInteraction.ID,
	}))
	claimed, err = suite.db.RecoverFailedPrompt(ctx, session.ID, eligiblePromptID, eligibleInteraction.ID)
	suite.Require().NoError(err)
	suite.True(claimed)
	prompt, err := suite.db.GetPromptHistoryEntry(ctx, eligiblePromptID)
	suite.Require().NoError(err)
	suite.Equal("pending", prompt.Status)
	updatedSession, err := suite.db.GetSession(ctx, session.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(updatedSession.Metadata.Recovery)
	suite.Equal(eligiblePromptID, updatedSession.Metadata.Recovery.PromptID)
	cleared, err = suite.db.ClearSessionProviderRecovery(ctx, session.ID, eligiblePromptID, eligibleInteraction.ID)
	suite.Require().NoError(err)
	suite.True(cleared)
	unchangedInteraction, err := suite.db.GetInteraction(ctx, eligibleInteraction.ID)
	suite.Require().NoError(err)
	suite.Equal(types.InteractionStateError, unchangedInteraction.State)
}

// TestGetNextPendingPrompt_RetryCapExcludesRunawayFailures verifies the runaway
// guard: a failed prompt whose retry_count has reached the cap is no longer
// selected by the queue, but is selected again if the cap is raised. Guards the
// infinite-redispatch flood in design/2026-06-15-wedged-acp-thread-autowake-flood.md.
func (suite *PostgresStoreTestSuite) TestGetNextPendingPrompt_RetryCapExcludesRunawayFailures() {
	ctx := context.Background()
	sessionID := "ses_retrycap_" + system.GenerateUUID()
	past := time.Now().Add(-time.Hour)

	mkFailed := func(id string, retryCount int) *types.PromptHistoryEntry {
		return &types.PromptHistoryEntry{
			ID:          id,
			UserID:      "user_retrycap",
			ProjectID:   "prj_retrycap",
			SpecTaskID:  "spt_retrycap",
			SessionID:   sessionID,
			Content:     "do the thing " + id,
			Status:      "failed",
			RetryCount:  retryCount,
			NextRetryAt: &past,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	}

	capped := mkFailed("phe_capped_"+system.GenerateUUID(), defaultMaxPromptQueueRetries)
	suite.Require().NoError(suite.db.gdb.WithContext(ctx).Create(capped).Error)
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM prompt_history_entries WHERE session_id = ?", sessionID)
	})

	// Default cap: the capped prompt is excluded → nothing to dispatch.
	got, err := suite.db.GetNextPendingPrompt(ctx, sessionID)
	suite.Require().NoError(err)
	suite.Nil(got, "a prompt at the retry cap must not be re-selected")

	// A fresh failed prompt (retry_count below cap) IS selected.
	fresh := mkFailed("phe_fresh_"+system.GenerateUUID(), 0)
	suite.Require().NoError(suite.db.gdb.WithContext(ctx).Create(fresh).Error)
	got, err = suite.db.GetNextPendingPrompt(ctx, sessionID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got)
	suite.Equal(fresh.ID, got.ID, "the below-cap prompt should be selected, not the capped one")

	// Raising the cap re-enables the previously-capped prompt.
	suite.T().Setenv("HELIX_MAX_PROMPT_QUEUE_RETRIES", "1000")
	got, err = suite.db.GetNextPendingPrompt(ctx, sessionID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got, "raising the cap must re-enable the previously-capped prompt")
	suite.Equal(capped.ID, got.ID)
}

func (suite *PostgresStoreTestSuite) TestRequeueBouncedPrompt_UpdatesExactInFlightPrompt() {
	ctx := context.Background()
	sessionID := "ses_bounce_" + system.GenerateUUID()
	now := time.Now()

	older := &types.PromptHistoryEntry{
		ID:        "phe_older_" + system.GenerateUUID(),
		UserID:    "user_bounce",
		SessionID: sessionID,
		Content:   "older prompt",
		Status:    "sent",
		CreatedAt: now.Add(-time.Minute),
	}
	newer := &types.PromptHistoryEntry{
		ID:        "phe_newer_" + system.GenerateUUID(),
		UserID:    "user_bounce",
		SessionID: sessionID,
		Content:   "newer prompt",
		Status:    "sent",
		CreatedAt: now,
	}
	pending := &types.PromptHistoryEntry{
		ID:        "phe_pending_" + system.GenerateUUID(),
		UserID:    "user_bounce",
		SessionID: sessionID,
		Content:   "pending prompt",
		Status:    "pending",
		CreatedAt: now.Add(time.Minute),
	}
	suite.Require().NoError(suite.db.gdb.WithContext(ctx).Create([]*types.PromptHistoryEntry{older, newer, pending}).Error)
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM prompt_history_entries WHERE session_id = ?", sessionID)
	})

	suite.Require().NoError(suite.db.RequeueBouncedPrompt(ctx, older.ID))
	suite.Require().Error(suite.db.RequeueBouncedPrompt(ctx, pending.ID))

	for _, expected := range []struct {
		id         string
		status     string
		retryCount int
	}{
		{older.ID, "failed", 1},
		{newer.ID, "sent", 0},
		{pending.ID, "pending", 0},
	} {
		got, err := suite.db.GetPromptHistoryEntry(ctx, expected.id)
		suite.Require().NoError(err)
		suite.Require().NotNil(got)
		suite.Equal(expected.status, got.Status)
		suite.Equal(expected.retryCount, got.RetryCount)
	}
}

func (suite *PostgresStoreTestSuite) TestResetCrashedPromptsForSession() {
	ctx := context.Background()
	sessionID := "ses_crashed_" + system.GenerateUUID()
	otherSessionID := "ses_other_crashed_" + system.GenerateUUID()
	now := time.Now()
	past := now.Add(-time.Minute)
	deletedAt := now

	target := &types.PromptHistoryEntry{
		ID: "phe_crashed_" + system.GenerateUUID(), UserID: "user_crashed", SessionID: sessionID,
		Content: "retry after config repair", Status: "pending", RetryCount: 4, CreatedAt: now.Add(-4 * time.Minute),
	}
	ordinaryFailure := &types.PromptHistoryEntry{
		ID: "phe_failed_" + system.GenerateUUID(), UserID: "user_crashed", SessionID: sessionID,
		Content: "ordinary failure", Status: "failed", RetryCount: 2, NextRetryAt: &past,
		ErrorMessage: "temporary failure", CreatedAt: now.Add(-3 * time.Minute),
	}
	deleted := &types.PromptHistoryEntry{
		ID: "phe_deleted_crashed_" + system.GenerateUUID(), UserID: "user_crashed", SessionID: sessionID,
		Content: "deleted crash", Status: "pending", DeletedAt: &deletedAt, CreatedAt: now.Add(-5 * time.Minute),
	}
	otherSession := &types.PromptHistoryEntry{
		ID: "phe_other_crashed_" + system.GenerateUUID(), UserID: "user_crashed", SessionID: otherSessionID,
		Content: "other session crash", Status: "pending", RetryCount: 3, CreatedAt: now.Add(-2 * time.Minute),
	}
	suite.Require().NoError(suite.db.gdb.WithContext(ctx).Create(
		[]*types.PromptHistoryEntry{target, ordinaryFailure, deleted, otherSession},
	).Error)
	suite.T().Cleanup(func() {
		suite.db.gdb.Exec("DELETE FROM prompt_history_entries WHERE session_id IN ?", []string{sessionID, otherSessionID})
	})

	for _, entry := range []*types.PromptHistoryEntry{target, deleted, otherSession} {
		suite.Require().NoError(suite.db.MarkPromptAsCrashed(ctx, entry.ID, "agent startup failed"))
	}

	resetCount, err := suite.db.ResetCrashedPromptsForSession(ctx, sessionID)
	suite.Require().NoError(err)
	suite.Equal(1, resetCount)

	got, err := suite.db.GetPromptHistoryEntry(ctx, target.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(got)
	suite.Equal("pending", got.Status)
	suite.Zero(got.RetryCount)
	suite.Nil(got.NextRetryAt)
	suite.Empty(got.ErrorMessage)

	got, err = suite.db.GetPromptHistoryEntry(ctx, ordinaryFailure.ID)
	suite.Require().NoError(err)
	suite.Equal("failed", got.Status)
	suite.Equal(2, got.RetryCount)
	suite.Equal("temporary failure", got.ErrorMessage)
	suite.WithinDuration(past, *got.NextRetryAt, time.Second)

	for _, entry := range []*types.PromptHistoryEntry{deleted, otherSession} {
		got, err = suite.db.GetPromptHistoryEntry(ctx, entry.ID)
		suite.Require().NoError(err)
		suite.Equal("failed", got.Status)
		suite.Equal("agent startup failed", got.ErrorMessage)
		suite.WithinDuration(crashedPromptRetrySentinel, *got.NextRetryAt, time.Second)
	}

	claimed, err := suite.db.GetAnyPendingPrompt(ctx, sessionID)
	suite.Require().NoError(err)
	suite.Require().NotNil(claimed)
	suite.Equal(target.ID, claimed.ID)
	suite.Equal("sending", claimed.Status)
}
