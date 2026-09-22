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

// transcriptElisionNotice marks the gap middle-out truncation leaves behind, so
// the model knows the middle is missing rather than inferring a discontinuity.
const transcriptElisionNotice = "\n\n[Note: the middle of this transcript was elided to fit the context limit. " +
	"The earliest turns (framing, constraints, rejected approaches) and the most recent turns are preserved.]\n\n"

const transcriptBlockSeparator = "\n\n"

// transcriptTruncationMode selects which end of an over-budget transcript to
// keep.
type transcriptTruncationMode int

const (
	// truncateOldestFirst drops the oldest turns. Correct for a generic agent
	// switch, where the user's latest intent is what matters.
	truncateOldestFirst transcriptTruncationMode = iota
	// truncateMiddleOut keeps both ends. Correct for a planning→implementation
	// handoff, where the earliest turns hold the framing, the constraints and
	// the rejected approaches — which the approved specs do not record.
	truncateMiddleOut
)

func (m transcriptTruncationMode) String() string {
	if m == truncateMiddleOut {
		return "middle_out"
	}
	return "oldest_first"
}

// transcriptStats explains what serialization did, so an empty or truncated
// result is not indistinguishable from "there was nothing to send".
type transcriptStats struct {
	blocks             int
	skippedNotComplete int
	skippedForkMarkers int
	originalBytes      int
	finalBytes         int
	truncated          bool
	blocksDropped      int
}

// serializeTranscript turns a chronological list of interactions into a markdown
// transcript suitable for seeding a forked session's new agent, dropping the
// oldest turns when over budget. See serializeTranscriptWithMode.
func serializeTranscript(interactions []*types.Interaction, maxBytes int) string {
	transcript, _ := serializeTranscriptWithMode(interactions, maxBytes, truncateOldestFirst)
	return transcript
}

// serializeTranscriptWithMode is serializeTranscript with an explicit
// truncation mode and a stats report. The output is stored once on the child's
// fork_seed.ResponseMessage at fork time and re-read by maybePrependTranscript
// when the first real user message goes out.
//
// Skips:
//   - fork_seed / fork_handoff interactions (the seed itself shouldn't
//     recursively appear; the handoff is meta-prompt, not conversation)
//   - interactions in non-Complete state (partial / errored turns add noise)
func serializeTranscriptWithMode(
	interactions []*types.Interaction,
	maxBytes int,
	mode transcriptTruncationMode,
) (string, transcriptStats) {
	var stats transcriptStats
	if len(interactions) == 0 {
		return "", stats
	}

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
			stats.skippedForkMarkers++
			continue
		}
		if in.State != types.InteractionStateComplete {
			stats.skippedNotComplete++
			continue
		}
		block := serializeInteractionBlock(in)
		if block == "" {
			continue
		}
		blocks = append(blocks, block)
	}
	stats.blocks = len(blocks)

	if len(blocks) == 0 {
		return "", stats
	}

	transcript := strings.Join(blocks, transcriptBlockSeparator)
	stats.originalBytes = len(transcript)
	stats.finalBytes = len(transcript)
	if maxBytes <= 0 || len(transcript) <= maxBytes {
		return transcript, stats
	}

	stats.truncated = true
	if mode == truncateMiddleOut {
		transcript, stats.blocksDropped = truncateTranscriptMiddleOut(blocks, maxBytes-len(transcriptElisionNotice))
	} else {
		kept := len(blocks)
		// Drop oldest blocks until we fit, then prepend the notice.
		for len(blocks) > 1 && len(transcript) > maxBytes-len(transcriptTruncationNotice) {
			blocks = blocks[1:]
			transcript = strings.Join(blocks, transcriptBlockSeparator)
		}
		stats.blocksDropped = kept - len(blocks)
		transcript = transcriptTruncationNotice + transcript
		// Final hard cap: a single huge block can still exceed the limit;
		// truncate the head of the block content rather than dropping it.
		if len(transcript) > maxBytes {
			transcript = transcriptTruncationNotice + transcript[len(transcript)-(maxBytes-len(transcriptTruncationNotice)):]
		}
	}
	stats.finalBytes = len(transcript)

	// Truncation was completely invisible before: that is how a transcript at
	// 98% of the ceiling went unnoticed.
	log.Warn().
		Int("original_bytes", stats.originalBytes).
		Int("final_bytes", stats.finalBytes).
		Int("max_bytes", maxBytes).
		Int("blocks", stats.blocks).
		Int("blocks_dropped", stats.blocksDropped).
		Str("mode", mode.String()).
		Msg("transcript seed exceeded the byte budget and was truncated")

	return transcript, stats
}

// truncateTranscriptMiddleOut keeps the head and tail of a block list within
// budget bytes and elides the middle. Whole blocks are preferred, but a
// planning session is typically one to three very large blocks, so block
// selection alone would never fit — when a single block exceeds its half of the
// budget it is cut at the byte level. Returns the transcript and how many whole
// blocks were dropped.
func truncateTranscriptMiddleOut(blocks []string, budget int) (string, int) {
	if budget <= 0 || len(blocks) == 0 {
		return transcriptElisionNotice, len(blocks)
	}
	headBudget := budget / 2
	tailBudget := budget - headBudget

	head, headUsed := 0, 0
	for head < len(blocks) {
		cost := len(blocks[head])
		if head > 0 {
			cost += len(transcriptBlockSeparator)
		}
		if headUsed+cost > headBudget {
			break
		}
		headUsed += cost
		head++
	}
	tail, tailUsed := len(blocks), 0
	for tail > head {
		cost := len(blocks[tail-1])
		if tail < len(blocks) {
			cost += len(transcriptBlockSeparator)
		}
		if tailUsed+cost > tailBudget {
			break
		}
		tailUsed += cost
		tail--
	}

	headText := strings.Join(blocks[:head], transcriptBlockSeparator)
	tailText := strings.Join(blocks[tail:], transcriptBlockSeparator)

	// Neither end fitted a whole block — a single oversized block. Cut bytes so
	// the framing at the top and the latest state at the bottom both survive.
	if headText == "" && head < len(blocks) {
		headText = blocks[head][:minInt(headBudget, len(blocks[head]))]
	}
	if tailText == "" && tail > 0 {
		last := blocks[tail-1]
		tailText = last[len(last)-minInt(tailBudget, len(last)):]
	}
	return headText + transcriptElisionNotice + tailText, tail - head
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	wroteEntry := false
	for _, e := range entries {
		if e.Type == "plan" {
			continue
		}
		if wroteEntry {
			b.WriteString("\n\n")
		}
		wroteEntry = true
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
// NOT wired into the reconnect resume path: that path delivers an
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
	for i := len(interactions) - 1; i >= 0; i-- {
		in := interactions[i]
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
	// Cheap precondition: only forked sessions (parent_session_id) or
	// in-place agent switches (agent_switched_at) carry a fork_seed
	// interaction. Skipping the DB lookup on regular sessions avoids an
	// extra ListInteractions call on every first message of every session —
	// only sessions that were forked or switched pay it.
	if session.Metadata.ParentSessionID == "" && session.Metadata.AgentSwitchedAt.IsZero() {
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
