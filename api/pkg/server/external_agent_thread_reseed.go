package server

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/helixml/helix/api/pkg/hydra"
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

// reseedBytesPerContextToken sizes the seed against the model's own window
// when that window is known: about half of it, at ~4 bytes per token, so the
// agent's system prompt, tools and its reply still fit. maxTranscriptBytes
// (sized for 200k windows) stays the ceiling.
const reseedBytesPerContextToken = 2

// buildThreadReseedPreamble renders the context an agent needs to carry on in a
// fresh thread after its previous one was lost — the workspace holding the
// agent's session store was recreated, the agent reported the thread missing,
// or the agent cannot reopen sessions at all.
//
// The agent's memory of the thread is gone; Helix holds the authoritative
// transcript and (for a spec task) the goal, branch and design docs. The
// transcript is serialized exactly as an agent switch does
// (serializeTranscriptWithMode), within reseedBudget.
//
// currentInteractionID is the turn about to be sent right after the seed; it is
// excluded so the agent does not read its pending question as already asked.
//
// Returns "" when there is nothing worth seeding — a first turn, or a session
// with no task and no history — so the caller sends the user's message alone.
func (apiServer *HelixAPIServer) buildThreadReseedPreamble(ctx context.Context, session *types.Session, currentInteractionID string) (string, transcriptStats) {
	if session == nil {
		return "", transcriptStats{}
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

	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		PerPage:      10_000,
	})
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).
			Msg("thread reseed: could not list interactions for context preamble")
	}
	interactions = slices.DeleteFunc(interactions, func(in *types.Interaction) bool {
		return in == nil || in.ID == currentInteractionID
	})
	transcript, stats := serializeTranscriptWithMode(interactions, apiServer.reseedBudget(ctx, session), truncateOldestFirst)
	if task == nil && transcript == "" {
		return "", stats
	}

	var b strings.Builder
	b.WriteString("[Helix: your previous agent session for this conversation was lost, ")
	b.WriteString("so you are continuing in a fresh session. ")
	b.WriteString("The workspace on disk may differ from what the conversation below describes — ")
	b.WriteString("re-read files before relying on them.]\n")

	if task != nil {
		b.WriteString("\n## Task\n")
		if task.Name != "" {
			fmt.Fprintf(&b, "%s\n", task.Name)
		}
		if task.OriginalPrompt != "" {
			fmt.Fprintf(&b, "\nOriginal request:\n%s\n", task.OriginalPrompt)
		}
		if task.BranchName != "" {
			fmt.Fprintf(&b, "\nBranch: %s\n", task.BranchName)
		}
		if task.DesignDocPath != "" {
			fmt.Fprintf(&b, "Design docs: %s\n", task.DesignDocPath)
		}
	}

	if transcript != "" {
		b.WriteString("\n## Conversation so far\n\n")
		b.WriteString(transcript)
		b.WriteString("\n")
	}

	b.WriteString("\n---\n\nThe user's message follows.\n\n")

	return b.String(), stats
}

// reseedBudget caps the seed transcript at maxTranscriptBytes, or at about half
// the session's model window when that is smaller. The window comes from the
// same code-agent config the desktop is configured with (model catalogue,
// overridden by the provider's advertised max_model_len).
func (apiServer *HelixAPIServer) reseedBudget(ctx context.Context, session *types.Session) int {
	app, _ := apiServer.runtimeAppForSession(ctx, session)
	if app == nil {
		return maxTranscriptBytes
	}
	cfg := apiServer.buildCodeAgentConfig(ctx, app, hydra.SandboxAPIProxyURL, "")
	if cfg == nil {
		return maxTranscriptBytes
	}
	return reseedBudgetForContext(cfg.MaxTokens)
}

func reseedBudgetForContext(contextTokens int) int {
	if contextTokens <= 0 {
		return maxTranscriptBytes
	}
	return min(maxTranscriptBytes, contextTokens*reseedBytesPerContextToken)
}
