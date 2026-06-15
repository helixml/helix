package orgchart

import (
	"fmt"
	"strconv"
	"strings"
)

// WorkerKind distinguishes a human Worker from an AI Worker. The two
// are the only flavours today; the marker-method pattern on the
// Worker interface keeps the set closed.
type WorkerKind string

const (
	// WorkerKindHuman is a real person inside the organisation.
	WorkerKindHuman WorkerKind = "human"

	// WorkerKindAI is a software agent.
	WorkerKindAI WorkerKind = "ai"
)

// WorkerKindValues lists every valid WorkerKind in canonical display
// order (Human first, then AI). The order is part of the public
// surface — it appears in JSON Schema enum lists and in
// `(valid: …)` error messages.
func WorkerKindValues() []WorkerKind {
	return []WorkerKind{WorkerKindHuman, WorkerKindAI}
}

// Validate returns an error if k is not one of the known worker
// kinds. The error contains the offending value and lists every
// valid option in quotes, so a client that posted a bad value can
// self-correct without reading source.
func (k WorkerKind) Validate() error {
	for _, v := range WorkerKindValues() {
		if k == v {
			return nil
		}
	}
	return fmt.Errorf("unknown worker kind %q (valid: %s)", k, quotedKinds(WorkerKindValues()))
}

// quotedKinds renders a slice of WorkerKind values as a
// comma-separated list of quoted strings, e.g. `"human", "ai"`.
func quotedKinds(vals []WorkerKind) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Quote(string(v))
	}
	return strings.Join(parts, ", ")
}
