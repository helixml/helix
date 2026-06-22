package activation_test

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
)

// TestSegmentMarkerRoundTrip pins the wire shape of the canonical
// transcript-line format. Every emitter (helix Spawner's bridge,
// the owner-chat path) builds a TranscriptSegment and calls Marker()
// to render the on-topic string; every consumer can ParseSegment
// the body back into a typed segment. The format is asserted here so
// writers and readers can't drift.
func TestSegmentMarkerRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		in     activation.TranscriptSegment
		marker string
	}{
		{
			name:   "assistant text",
			in:     activation.TranscriptSegment{Kind: activation.SegmentAssistant, Body: "hello"},
			marker: "assistant: hello",
		},
		{
			name:   "tool_use carries tool name",
			in:     activation.TranscriptSegment{Kind: activation.SegmentToolUse, ToolName: "bash", Body: "ls -la"},
			marker: "tool_use bash: ls -la",
		},
		{
			name:   "tool_result",
			in:     activation.TranscriptSegment{Kind: activation.SegmentToolResult, Body: "done"},
			marker: "tool_result: done",
		},
		{
			name:   "tool_result-error",
			in:     activation.TranscriptSegment{Kind: activation.SegmentToolResultError, Body: "permission denied"},
			marker: "tool_result-error: permission denied",
		},
		{
			name:   "error",
			in:     activation.TranscriptSegment{Kind: activation.SegmentError, Body: "boom"},
			marker: "error: boom",
		},
		{
			name:   "user bubble (owner-chat path)",
			in:     activation.TranscriptSegment{Kind: activation.SegmentUser, Body: "what's the plan"},
			marker: "user: what's the plan",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.in.Marker(); got != tc.marker {
				t.Errorf("Marker() = %q, want %q", got, tc.marker)
			}
			parsed, ok := activation.ParseSegment(tc.marker)
			if !ok {
				t.Fatalf("ParseSegment(%q) = _, false; want true", tc.marker)
			}
			if parsed.Kind != tc.in.Kind {
				t.Errorf("parsed.Kind = %q, want %q", parsed.Kind, tc.in.Kind)
			}
			if parsed.ToolName != tc.in.ToolName {
				t.Errorf("parsed.ToolName = %q, want %q", parsed.ToolName, tc.in.ToolName)
			}
			if parsed.Body != tc.in.Body {
				t.Errorf("parsed.Body = %q, want %q", parsed.Body, tc.in.Body)
			}
		})
	}
}

// TestSegmentMarkerCollapsesWhitespace pins the historical behaviour
// of OneLine(text, 500) — multi-line and multi-whitespace inputs get
// collapsed to a single space so the transcript line stays one event.
// Truncation to 500 runes is also part of the format.
func TestSegmentMarkerCollapsesWhitespace(t *testing.T) {
	t.Parallel()
	s := activation.TranscriptSegment{Kind: activation.SegmentAssistant, Body: "one\n  two\tthree   four"}
	if got, want := s.Marker(), "assistant: one two three four"; got != want {
		t.Fatalf("Marker() = %q, want %q", got, want)
	}

	// Truncation: marker body capped to 500 chars + ellipsis.
	long := activation.TranscriptSegment{Kind: activation.SegmentAssistant, Body: strings.Repeat("x", 600)}
	got := long.Marker()
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("Marker() over-long input should end with '…': got %q", got)
	}
	// "assistant: " + 500 x + "…"
	if want := len("assistant: ") + 500 + len("…"); len(got) != want {
		t.Fatalf("Marker() length = %d, want %d", len(got), want)
	}
}

// TestParseSegmentRejectsNonSegment confirms that markers from other
// shapes (start/exit markers, plain text, unknown prefixes) parse to
// (_, false) without surprises.
func TestParseSegmentRejectsNonSegment(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"=== activation: hire ===",
		"=== exit: ok ===",
		"plain text",
		"tool_use: missing name", // tool_use requires a name token
	}
	for _, body := range cases {
		body := body
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			if got, ok := activation.ParseSegment(body); ok {
				t.Fatalf("ParseSegment(%q) = %+v, true; want false", body, got)
			}
		})
	}
}
