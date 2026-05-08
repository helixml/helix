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

// MessageSenderCINotifier wraps a SpecTaskMessageSender (already used by
// the design-review and approval flows) and delivers CI messages over
// the same path. This way agents that are offline fall through to the
// existing waiting-interaction queue without us having to reinvent that
// logic. Constructed by the API server, where the sender is wired up.
type MessageSenderCINotifier struct {
	sender SpecTaskMessageSender
}

// NewMessageSenderCINotifier returns a CINotifier that pushes CI messages
// through the given SpecTaskMessageSender. interrupt is always false:
// CI results aren't urgent enough to interrupt mid-turn — the agent picks
// them up at the next message-pump tick.
func NewMessageSenderCINotifier(sender SpecTaskMessageSender) *MessageSenderCINotifier {
	return &MessageSenderCINotifier{sender: sender}
}

// NotifyCIResult sends the CI message to the agent. notifyUserID is left
// empty — CI results aren't tied to a specific commenter.
func (n *MessageSenderCINotifier) NotifyCIResult(
	ctx context.Context,
	task *types.SpecTask,
	_ *types.RepoPR,
	message string,
) error {
	if n == nil || n.sender == nil {
		return nil
	}
	_, _, err := n.sender(ctx, task, message, "", false)
	return err
}
