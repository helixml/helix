package server

import (
	"context"

	"github.com/rs/zerolog/log"
)

// resetAbandonedWork fails or clears work left in flight by an API process
// that is gone, each time this process becomes leader. Only rows from before
// this process started are touched: during a blue/green deploy the new colour
// serves requests for a while before it gains the lease, and that work is its
// own.
func (apiServer *HelixAPIServer) resetAbandonedWork(ctx context.Context) {
	before := apiServer.startedAt

	if err := apiServer.Store.ResetRunningExecutions(ctx, before); err != nil {
		log.Error().Err(err).Msg("failed to reset running trigger executions")
	}
	if err := apiServer.Store.ResetRunningInteractions(ctx, before); err != nil {
		log.Error().Err(err).Msg("failed to reset running interactions")
	}

	// Sessions stuck in "starting"/"restarting": the process orchestrating
	// the start is gone.
	if cleaned, err := apiServer.Store.ClearStaleStartingSessions(ctx, before); err != nil {
		log.Error().Err(err).Msg("Failed to clear stale starting sessions")
	} else if cleaned > 0 {
		log.Info().Int64("cleaned", cleaned).Msg("Cleared stale 'starting' sessions from previous API crash")
	}

	// Catch up prompt_history_entries stuck in 'sending' because the old
	// in-memory interactionToPromptMapping was lost before MarkPromptAsSent
	// could fire (typically across an API restart). See
	// design/2026-04-30-queue-and-other-stuck-state-bugs.md. New rows
	// shouldn't get stuck — Interaction.PromptID persists the link — but this
	// also unsticks legacy rows from before that schema change. Only touches
	// prompts whose interaction completed, so it is safe at any time.
	fixed, err := apiServer.Store.ReconcileStuckSendingPrompts(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reconcile stuck 'sending' prompts at startup")
	} else if fixed > 0 {
		log.Info().Int("count", fixed).Msg("✅ [QUEUE] Reconciled stuck 'sending' prompts at startup (legacy in-memory-mapping orphans)")
	}
}
