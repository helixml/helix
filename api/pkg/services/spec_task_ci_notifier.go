package services

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// CINotifier delivers a CI result message to the agent currently working
// on a spec task. The orchestrator calls this from the PR poll loop once
// per (pr, transition) — see the dedup logic in handleCIStatusTransition.
//
// Implementations are expected to be best-effort: if the agent is
// offline, the message should be queued (e.g. as a waiting Interaction)
// for delivery when the agent reconnects, not failed loudly. Returning
// an error is fine for *unexpected* failures — the orchestrator logs
// them and continues with the next task.
type CINotifier interface {
	NotifyCIResult(ctx context.Context, task *types.SpecTask, repo *types.RepoPR, status string) error
}

// EnqueueCINotifier delivers CI messages through the session-scoped prompt
// queue (SpecTaskMessageEnqueuer). Constructed by the API server, where the
// enqueuer is wired up. Offline agents still fall through to the existing
// waiting-interaction queue — the enqueue path boots a stopped desktop and
// delivers on reconnect.
type EnqueueCINotifier struct {
	enqueue SpecTaskMessageEnqueuer
}

// NewEnqueueCINotifier returns a CINotifier that enqueues CI messages.
// interrupt is always true: a CI pass/fail is worth surfacing to the agent
// immediately (cancel the current turn, respecting the boot barrier, then send)
// so it learns of failing tests straight away, even mid-turn. CI results are not
// coalesced — each transition is delivered as its own interrupt.
func NewEnqueueCINotifier(enqueue SpecTaskMessageEnqueuer) *EnqueueCINotifier {
	return &EnqueueCINotifier{enqueue: enqueue}
}

// NotifyCIResult enqueues the CI message to the agent as an interrupt.
func (n *EnqueueCINotifier) NotifyCIResult(
	ctx context.Context,
	task *types.SpecTask,
	_ *types.RepoPR,
	message string,
) error {
	if n == nil || n.enqueue == nil {
		return nil
	}
	return n.enqueue(ctx, task, message, true, "")
}
