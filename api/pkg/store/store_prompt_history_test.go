package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

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
