package activation

import (
	"fmt"
	"strings"
)

// Status enumerates the terminal states an Activation can reach.
// Today there are exactly two — the Spawner either returns nil
// (ok) or an error (failed). Future statuses (e.g. cancelled,
// timed-out) would land here as new constants alongside.
type Status string

const (
	// StatusOK means the Spawner returned nil for this activation.
	StatusOK Status = "ok"

	// StatusError means the Spawner returned a non-nil error. The
	// error text lives on Outcome.Error.
	StatusError Status = "error"
)

// Outcome is the terminal result of one Activation. It is the typed
// replacement for the historical `=== exit: ok ===` /
// `=== exit: error: <msg> ===` marker strings that emitters publish
// as the last event on the transcript.
//
// Writers build an Outcome and call Marker() to render the on-wire
// string. Readers call ParseOutcomeMarker to recover the typed
// Outcome. Until the Activation aggregate (B5.4+) lands, the marker
// is still the primary record of completion — the row-based
// representation joins it later, not replaces it.
type Outcome struct {
	Status Status
	Error  string
}

// OutcomeFromError is the canonical adapter from a Spawner's return
// value to an Outcome. nil → StatusOK; anything else → StatusError
// with err.Error() captured.
func OutcomeFromError(err error) Outcome {
	if err == nil {
		return Outcome{Status: StatusOK}
	}
	return Outcome{Status: StatusError, Error: err.Error()}
}

// Marker renders the outcome to the canonical transcript-marker
// string. The format is part of the public contract and is asserted
// by outcome_test.go; do not change it without coordinating with
// every reader (chat UI, worker_log consumers, prompts that look at
// activation history).
func (o Outcome) Marker() string {
	switch o.Status {
	case StatusOK:
		return "=== exit: ok ==="
	case StatusError:
		return fmt.Sprintf("=== exit: error: %s ===", o.Error)
	default:
		// Unknown statuses get a defensive marker rather than a
		// panic so a future enum addition without an emitter update
		// is recoverable from the transcript.
		return fmt.Sprintf("=== exit: %s ===", o.Status)
	}
}

// ParseOutcomeMarker is the inverse of Outcome.Marker. Returns
// (_, false) for any body that isn't a recognised exit marker —
// callers can do `if out, ok := ParseOutcomeMarker(body); ok`
// without worrying about false positives on assistant text or sibling
// markers like `=== activation: hire ===`.
func ParseOutcomeMarker(body string) (Outcome, bool) {
	const prefix = "=== exit: "
	const suffix = " ==="
	if !strings.HasPrefix(body, prefix) || !strings.HasSuffix(body, suffix) {
		return Outcome{}, false
	}
	inner := body[len(prefix) : len(body)-len(suffix)]
	if inner == string(StatusOK) {
		return Outcome{Status: StatusOK}, true
	}
	if rest, ok := strings.CutPrefix(inner, "error: "); ok {
		return Outcome{Status: StatusError, Error: rest}, true
	}
	return Outcome{}, false
}
