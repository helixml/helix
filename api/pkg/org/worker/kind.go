package worker

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind distinguishes a human Worker from an AI Worker. The two are the
// only flavours today; the marker-method pattern on the Worker
// interface (still in helix-org/domain) keeps the set closed.
//
// Naming: lifted from helix-org/worker.Kind in B3a. The
// redundant "Worker" prefix drops because the package qualifier
// already supplies it (B1 precedent: transport.Kind).
type Kind string

const (
	// KindHuman is a real person inside the organisation. Human Workers
	// are not activated by the dispatcher; their actions come from a
	// chat surface or the UI.
	KindHuman Kind = "human"

	// KindAI is a software agent. AI Workers are activated by the
	// dispatcher on hire and on every subscribed-Stream event.
	KindAI Kind = "ai"
)

// KindValues lists every valid Kind in canonical display order
// (KindHuman first, then KindAI). The order is part of the public
// surface — it appears in JSON Schema enum lists and in `(valid: …)`
// error messages; the W1 characterisation test pins it.
func KindValues() []Kind {
	return []Kind{KindHuman, KindAI}
}

// Validate returns an error if k is not one of the known worker
// kinds. The error contains the offending value and lists every
// valid option in quotes, so a client that posted a bad value can
// self-correct without reading source. The W6 characterisation test
// pins this contract.
func (k Kind) Validate() error {
	for _, v := range KindValues() {
		if k == v {
			return nil
		}
	}
	return fmt.Errorf("unknown worker kind %q (valid: %s)", k, quotedKinds(KindValues()))
}

// quotedKinds renders a slice of Kind values as a comma-separated list
// of quoted strings, e.g. `"human", "ai"`. Inlined from
// helix-org/domain.QuotedList to keep this package self-contained —
// when more types in api/pkg/org/ start duplicating this helper,
// factor it out into a shared internal location (not before).
// (Mirrors the same decision in api/pkg/org/transport/transport.go.)
func quotedKinds(vals []Kind) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Quote(string(v))
	}
	return strings.Join(parts, ", ")
}
