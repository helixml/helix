package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// maxTranscriptBytes caps the seed transcript injected on the first message of
// a forked session. Sized to leave room for the user's own message plus the
// target agent's system prompt within typical 200k-token context windows.
// See design/tasks/002081_kickoff-mid-session/design.md.
const maxTranscriptBytes = 400_000

// transcriptTruncationNotice is prepended (visibly) when the parent transcript
// exceeds maxTranscriptBytes. The truncation drops oldest interactions first
// because newer ones are more likely to be load-bearing on the user's intent.
const transcriptTruncationNotice = "[Note: earlier turns truncated to fit context limit.]\n\n"

// serializeTranscript turns a chronological list of interactions into a markdown
// transcript suitable for seeding a forked session's new agent. The output is
// stored once on the child's fork_seed.ResponseMessage at fork time and re-read
// by maybePrependTranscript when the first real user message goes out.
//
// Skips:
//   - fork_seed interactions (the seed itself shouldn't recursively appear)
//   - interactions in non-Complete state (partial / errored turns add noise)
//
// Truncates from the *front* when the byte budget is exceeded so the most
// recent context (and the user's latest intent) wins.
func serializeTranscript(interactions []*types.Interaction, maxBytes int) string {
	if len(interactions) == 0 {
		return ""
	}

	// Build per-interaction blocks first so we can truncate from the front.
	blocks := make([]string, 0, len(interactions))
	for _, in := range interactions {
		if in == nil {
			continue
		}
		// Skip both fork markers — fork_seed is the divider on the
		// parent (its ResponseMessage is the previous transcript blob,
		// already represented by the parent's inherited rows above
		// it), and fork_handoff is the synthetic warm-up turn (its
		// content is meta-prompt, not real conversation).
		if in.Trigger == types.InteractionTriggerForkSeed ||
			in.Trigger == types.InteractionTriggerForkHandoff {
			continue
		}
		if in.State != types.InteractionStateComplete {
			continue
		}
		block := serializeInteractionBlock(in)
		if block == "" {
			continue
		}
		blocks = append(blocks, block)
	}

	if len(blocks) == 0 {
		return ""
	}

	transcript := strings.Join(blocks, "\n\n")
	if maxBytes > 0 && len(transcript) > maxBytes {
		// Drop oldest blocks until we fit, then prepend the notice.
		for len(blocks) > 1 && len(transcript) > maxBytes-len(transcriptTruncationNotice) {
			blocks = blocks[1:]
			transcript = strings.Join(blocks, "\n\n")
		}
		transcript = transcriptTruncationNotice + transcript
		// Final hard cap: a single huge block can still exceed the limit;
		// truncate the head of the block content rather than dropping it.
		if len(transcript) > maxBytes {
			transcript = transcriptTruncationNotice + transcript[len(transcript)-(maxBytes-len(transcriptTruncationNotice)):]
		}
	}
	return transcript
}

// serializeInteractionBlock formats one complete interaction as a "**User:** …"
// + "**Assistant:** …" block. Returns "" when both sides are empty.
func serializeInteractionBlock(in *types.Interaction) string {
	user := strings.TrimSpace(in.PromptMessage)
	assistant := strings.TrimSpace(serializeAgentResponse(in))
	if user == "" && assistant == "" {
		return ""
	}
	var b strings.Builder
	if user != "" {
		b.WriteString("**User:** ")
		b.WriteString(user)
	}
	if assistant != "" {
		if user != "" {
			b.WriteString("\n\n")
		}
		b.WriteString("**Assistant:** ")
		b.WriteString(assistant)
	}
	return b.String()
}

// serializeAgentResponse renders the agent's side of one interaction.
// Prefers the structured ResponseEntries (which preserves text/tool_call
// boundaries) and degrades to the flat ResponseMessage when entries are
// absent (older interactions).
func serializeAgentResponse(in *types.Interaction) string {
	if in == nil {
		return ""
	}
	if len(in.ResponseEntries) == 0 {
		return in.ResponseMessage
	}
	var entries []wsprotocol.ResponseEntry
	if err := json.Unmarshal(in.ResponseEntries, &entries); err != nil || len(entries) == 0 {
		return in.ResponseMessage
	}

	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n\n")
		}
		switch e.Type {
		case "tool_call":
			name := e.ToolName
			if name == "" {
				name = "tool"
			}
			status := e.ToolStatus
			if status == "" {
				b.WriteString(fmt.Sprintf("[%s]", name))
			} else {
				b.WriteString(fmt.Sprintf("[%s: %s]", name, status))
			}
			if c := strings.TrimSpace(e.Content); c != "" {
				b.WriteString("\n")
				b.WriteString(c)
			}
		default: // "text" and any future plain-prose types
			b.WriteString(e.Content)
		}
	}
	return b.String()
}

// requireUnpaused short-circuits a request when the session is paused.
// Returns HTTP 409 with a clear reason so the frontend can render an
// actionable error (e.g. "fork from descendant instead"). Returns nil
// when the session is live.
//
// NOT wired into pickupWaitingInteraction: that path delivers an
// already-Waiting interaction to a freshly-connected agent, which the
// design explicitly preserves ("in-flight waiting interaction allowed
// to complete naturally — pausing is no-new-input, not kill-the-agent").
// Blocking pickup would strand the interaction permanently.
func requireUnpaused(session *types.Session) *system.HTTPError {
	if session == nil || !session.Metadata.Paused {
		return nil
	}
	reason := session.Metadata.PausedReason
	if reason == "" {
		reason = "paused"
	}
	return system.NewHTTPError409(fmt.Sprintf("session is paused (reason: %s)", reason))
}

// findForkSeed scans a session's interactions for the synthetic fork_seed
// marker created at fork time. Returns nil if absent (i.e. this session was
// not created by forking).
func findForkSeed(interactions []*types.Interaction) *types.Interaction {
	for _, in := range interactions {
		if in == nil {
			continue
		}
		if in.Trigger == types.InteractionTriggerForkSeed {
			return in
		}
	}
	return nil
}

// maybePrependTranscript injects the parent session's serialized transcript
// (captured at fork time on the fork_seed interaction) into the first
// outgoing user message of a forked session. Returns the (possibly modified)
// message unchanged when:
//   - the session has already opened its Zed thread (ZedThreadID != ""), OR
//   - the session has no fork_seed interaction (i.e. wasn't forked).
//
// The seed is only injected once per forked session, on the first message
// that creates the thread. After that, the agent has the context in its own
// thread state and subsequent messages flow normally.
func (apiServer *HelixAPIServer) maybePrependTranscript(ctx context.Context, session *types.Session, message string) string {
	if session == nil || session.Metadata.ZedThreadID != "" {
		return message
	}
	// Cheap precondition: only forked sessions have a parent_session_id (set
	// at fork time alongside the fork_seed interaction). Skipping the DB
	// lookup on regular sessions avoids an extra ListInteractions call on
	// every first message of every session — only forked sessions pay it.
	if session.Metadata.ParentSessionID == "" {
		return message
	}
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		PerPage:      1000,
	})
	if err != nil {
		log.Warn().Err(err).
			Str("session_id", session.ID).
			Msg("fork seed: failed to list interactions; sending message without seed")
		return message
	}
	seed := findForkSeed(interactions)
	if seed == nil {
		return message
	}
	transcript := strings.TrimSpace(seed.ResponseMessage)
	if transcript == "" {
		return message
	}
	log.Info().
		Str("session_id", session.ID).
		Str("parent_session_id", session.Metadata.ParentSessionID).
		Int("transcript_len", len(transcript)).
		Int("user_message_len", len(message)).
		Msg("fork seed: prepending parent transcript to first outgoing message")
	var b strings.Builder
	b.WriteString("The following is the transcript of a prior session that this conversation continues from. Treat it as background context; respond to the new user message that follows.\n\n---\n\n")
	b.WriteString(transcript)
	b.WriteString("\n\n---\n\n")
	b.WriteString(message)
	return b.String()
}
