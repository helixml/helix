package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// unrestorableThreadMarker is what Zed reports when the ACP agent backing a
// thread declares no `loadSession` capability in its `initialize` response
// (see zed/crates/external_websocket_sync/src/thread_service.rs, which checks
// AgentConnection::supports_load_session before every load).
//
// DeepSeek Harness is the first runtime Helix ships that answers this way: its
// ACP server is a separate demo composition whose capabilities are only
// `promptCapabilities`, with neither `loadSession` nor `sessionCapabilities.resume`.
// No amount of retrying makes such a load succeed — the protocol has no verb
// for re-entering the agent's session — so a thread whose Zed entity is gone
// (any Zed restart clears the in-memory registry) can never be reopened.
//
// Left untreated this is terminal, not cosmetic: the send path in Zed reports
// the failure and drops the message rather than silently starting a new thread,
// so every message after a restart vanishes and the task must be recreated.
const unrestorableThreadMarker = "does not support session loading"

// isUnrestorableThreadError reports whether a thread_load_error means the agent
// can never restore this thread, as opposed to a load that failed this time.
// The distinction decides the response: a transient failure is worth surfacing
// to the user, a permanent one is worth recovering from.
func isUnrestorableThreadError(errMsg string) bool {
	return strings.Contains(errMsg, unrestorableThreadMarker)
}

// Bounds on the replayed transcript. The seed rides in front of the user's own
// message on a turn that is already retrying, so it must stay small enough to
// be worth sending on every restart: enough for the agent to know what it was
// doing, not a second copy of the conversation.
const (
	reseedMaxTurns        = 6
	reseedMaxCharsPerTurn = 700
	reseedMaxTotalChars   = 6000
)

// buildThreadReseedPreamble renders the context an agent needs to carry on in a
// fresh session after its previous one became unrestorable.
//
// The agent's own memory died with its process and the protocol offers no way
// to recover it, so the choice is between a fresh session that knows what it
// was doing and one that does not. Helix holds the authoritative transcript and
// (for a spec task) the goal, branch and workspace, which is why the seed is
// composed here rather than in Zed.
//
// Returns "" when there is nothing worth seeding — a first turn, or a session
// with no task and no history — so the caller sends the user's message alone.
func (apiServer *HelixAPIServer) buildThreadReseedPreamble(ctx context.Context, session *types.Session, currentInteractionID string) string {
	if session == nil {
		return ""
	}

	var task *types.SpecTask
	if session.Metadata.SpecTaskID != "" {
		var err error
		task, err = apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
		if err != nil {
			// Best-effort: the transcript alone is still worth seeding, and a
			// turn that is already recovering must not fail over missing prose.
			log.Warn().Err(err).
				Str("session_id", session.ID).
				Str("spec_task_id", session.Metadata.SpecTaskID).
				Msg("thread reseed: could not load spec task for context preamble")
			task = nil
		}
	}

	transcript := apiServer.recentTranscript(ctx, session.ID, currentInteractionID)
	if task == nil && transcript == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("[Helix: your previous session ended and this agent cannot resume it, ")
	b.WriteString("so you are continuing in a fresh session. ")
	b.WriteString("The workspace on disk is unchanged — any work already committed or written is still there. ")
	b.WriteString("Re-read files before relying on them; the summary below is all that survived.]\n")

	if task != nil {
		b.WriteString("\n## Task\n")
		if task.Name != "" {
			fmt.Fprintf(&b, "%s\n", task.Name)
		}
		if task.OriginalPrompt != "" {
			fmt.Fprintf(&b, "\nOriginal request:\n%s\n", truncate(task.OriginalPrompt, reseedMaxCharsPerTurn))
		}
		if task.BranchName != "" {
			fmt.Fprintf(&b, "\nBranch: %s\n", task.BranchName)
		}
		if task.DesignDocPath != "" {
			fmt.Fprintf(&b, "Design docs: %s\n", task.DesignDocPath)
		}
	}

	if transcript != "" {
		b.WriteString("\n## Conversation so far\n")
		b.WriteString(transcript)
	}

	b.WriteString("\n---\n\nThe user's message follows.\n\n")

	return truncate(b.String(), reseedMaxTotalChars)
}

// recentTranscript renders the tail of the conversation as plain dialogue.
//
// The turn being replayed is excluded: it is sent immediately after the seed,
// and including it would have the agent read its own pending question as
// something already asked and answered.
func (apiServer *HelixAPIServer) recentTranscript(ctx context.Context, sessionID, currentInteractionID string) string {
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    sessionID,
		GenerationID: -1,
	})
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).
			Msg("thread reseed: could not list interactions for context preamble")
		return ""
	}

	var turns []string
	for _, interaction := range interactions {
		if interaction == nil || interaction.ID == currentInteractionID {
			continue
		}
		var turn strings.Builder
		if prompt := strings.TrimSpace(interaction.PromptMessage); prompt != "" {
			fmt.Fprintf(&turn, "User: %s\n", truncate(prompt, reseedMaxCharsPerTurn))
		}
		if response := strings.TrimSpace(interaction.ResponseMessage); response != "" {
			fmt.Fprintf(&turn, "Assistant: %s\n", truncate(response, reseedMaxCharsPerTurn))
		}
		if turn.Len() > 0 {
			turns = append(turns, turn.String())
		}
	}

	// Keep the most recent turns: the tail is what the agent was in the middle
	// of, and the head is the part the task description already covers.
	if len(turns) > reseedMaxTurns {
		turns = turns[len(turns)-reseedMaxTurns:]
	}

	return strings.Join(turns, "\n")
}

// truncate shortens s to at most max characters, marking where it was cut so
// the agent can tell an elided message from a short one.
func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	const ellipsis = "… [truncated]"
	if max <= len(ellipsis) {
		return s[:max]
	}
	return s[:max-len(ellipsis)] + ellipsis
}
