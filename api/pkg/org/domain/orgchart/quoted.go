package orgchart

import (
	"strconv"
	"strings"
)

// QuotedList renders a slice of string-typed values as a
// comma-separated list of quoted strings, e.g. `"human", "ai"`. The
// output is sized for a validation-error message that follows
// "(valid: …)" so callers can drop it inline without further
// formatting.
//
// The generic constraint accepts any string-derived type — WorkerKind,
// transport.Kind, tool.Name — so a single helper covers every "what
// are the valid values?" formatting site without each domain type
// needing its own bespoke list-formatter.
//
// Lifted from api/pkg/org/domain/enum.go in the DDD restructure.
// Lives in orgchart because it's a one-line helper used by every
// "Validate()" method that wants to list valid enum values; giving it
// its own package would be over-engineering.
func QuotedList[T ~string](vals []T) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Quote(string(v))
	}
	return strings.Join(parts, ", ")
}
