package activation_test

import (
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/activation"
)

// TestOutcomeMarkerRoundTrip pins both directions of the canonical
// "=== exit: ... ===" marker shape: how emitters render an Outcome
// to the transcript stream, and how readers parse the same marker
// back into a typed Outcome. The marker is the only wire format
// historically — every transcript reader (worker_log consumers, chat
// UI, /ui/streams) string-matched it. Lifting the format into one
// shared helper means writers and readers can't drift.
func TestOutcomeMarkerRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		in     activation.Outcome
		marker string
	}{
		{
			name:   "ok",
			in:     activation.Outcome{Status: activation.StatusOK},
			marker: "=== exit: ok ===",
		},
		{
			name:   "error with message",
			in:     activation.Outcome{Status: activation.StatusError, Error: "boom"},
			marker: "=== exit: error: boom ===",
		},
		{
			name:   "error preserves multi-word message",
			in:     activation.Outcome{Status: activation.StatusError, Error: "context deadline exceeded"},
			marker: "=== exit: error: context deadline exceeded ===",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.in.Marker(); got != tc.marker {
				t.Errorf("Marker() = %q, want %q", got, tc.marker)
			}
			parsed, ok := activation.ParseOutcomeMarker(tc.marker)
			if !ok {
				t.Fatalf("ParseOutcomeMarker(%q) = _, false; want true", tc.marker)
			}
			if parsed.Status != tc.in.Status {
				t.Errorf("parsed.Status = %q, want %q", parsed.Status, tc.in.Status)
			}
			if parsed.Error != tc.in.Error {
				t.Errorf("parsed.Error = %q, want %q", parsed.Error, tc.in.Error)
			}
		})
	}
}

// TestParseOutcomeMarkerRejectsNonMarker confirms that arbitrary
// transcript bodies (assistant text, tool calls, the start-of-activation
// marker, anything that isn't a `=== exit: …` line) parse cleanly
// to (Outcome{}, false). Callers can do `if out, ok := Parse...; ok`
// without false positives.
func TestParseOutcomeMarkerRejectsNonMarker(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"assistant: hello",
		"tool_use bash: ls",
		"=== activation: hire ===",   // sibling marker, must not match
		"=== exit:",                  // truncated
		"== exit: ok ===",            // wrong delimiter
		"=== exit: ok",               // missing trailing
		"=== exit: maybe ===",        // unknown status
		"prefix === exit: ok ===",    // not a pure marker
		"=== exit: ok === suffix",
	}
	for _, body := range cases {
		body := body
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			if got, ok := activation.ParseOutcomeMarker(body); ok {
				t.Fatalf("ParseOutcomeMarker(%q) = %+v, true; want false", body, got)
			}
		})
	}
}

// TestOutcomeFromError mirrors the spawner's only two outcomes today —
// nil → ok, anything else → error with .Error() captured. Keeps the
// emitter side from having to know the constants.
func TestOutcomeFromError(t *testing.T) {
	t.Parallel()
	if got := activation.OutcomeFromError(nil); got.Status != activation.StatusOK || got.Error != "" {
		t.Fatalf("OutcomeFromError(nil) = %+v, want ok with no error", got)
	}
	want := activation.Outcome{Status: activation.StatusError, Error: "boom"}
	if got := activation.OutcomeFromError(errors.New("boom")); got != want {
		t.Fatalf("OutcomeFromError(err) = %+v, want %+v", got, want)
	}
}
