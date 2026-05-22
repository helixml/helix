package activation

import (
	"fmt"
	"strings"
)

// SegmentKind enumerates the variants of activation-transcript line.
// The wire format is the prefix before ": " on the published Stream
// event body — every consumer matches on this prefix to render
// (chat UI) or summarise (worker_log).
type SegmentKind string

const (
	// SegmentAssistant is one chunk of the model's reply text.
	SegmentAssistant SegmentKind = "assistant"

	// SegmentToolUse is the model invoking a tool. The tool name is
	// carried on TranscriptSegment.ToolName, the call body on .Body.
	SegmentToolUse SegmentKind = "tool_use"

	// SegmentToolResult is a tool returning data successfully.
	SegmentToolResult SegmentKind = "tool_result"

	// SegmentToolResultError is a tool returning an error.
	SegmentToolResultError SegmentKind = "tool_result-error"

	// SegmentError is a runtime-emitted error that the Worker didn't
	// surface as a tool result (e.g. WS disconnect, decode failure).
	SegmentError SegmentKind = "error"

	// SegmentUser is a user-bubble line. Emitted by the owner-chat
	// path so the human's turns appear on s-activations-w-owner
	// alongside the assistant's.
	SegmentUser SegmentKind = "user"
)

// segmentBodyLimit caps the marker body at 500 runes plus an ellipsis
// — the long-standing convention every emitter has used (lifted
// from api/pkg/org/agent.OneLine). Keeping it inside Marker() means
// every emitter gets the same on-wire shape without each one having
// to remember to truncate.
const segmentBodyLimit = 500

// TranscriptSegment is one settled line on a Worker's activation
// transcript Stream — assistant text, tool call, tool result, etc.
// It is the typed shape that replaces the historical "prefix: body"
// strings invented per-emitter (`assistant: …`, `tool_use foo: …`).
// Writers build a segment and call Marker(); readers do the inverse
// via ParseSegment.
//
// Start-of-activation markers (`=== activation: … ===`) and end-of-
// activation markers (`=== exit: … ===`) are NOT segments — they
// live as their own VOs (Outcome already exists; a sibling for the
// start marker lands later).
type TranscriptSegment struct {
	Kind SegmentKind

	// ToolName is populated only when Kind == SegmentToolUse. It is
	// the canonical name of the tool the model invoked.
	ToolName string

	// Body is the segment's payload — assistant text, the tool call's
	// JSON arguments, the tool's stdout, etc. Free-form; Marker()
	// truncates and collapses whitespace before writing.
	Body string
}

// Marker renders the segment to its canonical on-wire string. The
// format is part of the public contract — see segment_test.go.
func (s TranscriptSegment) Marker() string {
	body := oneLine(s.Body, segmentBodyLimit)
	switch s.Kind {
	case SegmentToolUse:
		return fmt.Sprintf("tool_use %s: %s", s.ToolName, body)
	case SegmentAssistant,
		SegmentToolResult,
		SegmentToolResultError,
		SegmentError,
		SegmentUser:
		return string(s.Kind) + ": " + body
	default:
		// Defensive: unknown kind still produces a parseable line
		// rather than an empty string. Helps when a future kind is
		// added in the writer path before consumers update.
		return string(s.Kind) + ": " + body
	}
}

// ParseSegment is the inverse of Marker. Returns (_, false) for
// markers that aren't transcript-segment-shaped (start/exit markers,
// arbitrary text, tool_use without a tool name).
func ParseSegment(body string) (TranscriptSegment, bool) {
	// tool_use carries an extra "<name>: " token after the kind.
	if rest, ok := strings.CutPrefix(body, "tool_use "); ok {
		name, payload, found := strings.Cut(rest, ": ")
		if !found || name == "" {
			return TranscriptSegment{}, false
		}
		return TranscriptSegment{Kind: SegmentToolUse, ToolName: name, Body: payload}, true
	}
	// All other kinds: "<kind>: <body>".
	kindStr, payload, found := strings.Cut(body, ": ")
	if !found {
		return TranscriptSegment{}, false
	}
	kind := SegmentKind(kindStr)
	switch kind {
	case SegmentAssistant,
		SegmentToolResult,
		SegmentToolResultError,
		SegmentError,
		SegmentUser:
		return TranscriptSegment{Kind: kind, Body: payload}, true
	}
	return TranscriptSegment{}, false
}

// oneLine collapses whitespace and clips to max runes — the same
// shape api/pkg/org/agent.OneLine produces. Defined here so the
// activation package does not import agent.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}
