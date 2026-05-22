package domain

import (
	"strconv"
	"strings"
)

// QuotedList renders a slice of string-typed values as a comma-separated
// list of quoted strings, e.g. `"human", "ai"`. The output is sized for
// a validation-error message that follows "(valid: …)" so callers can
// drop it inline without further formatting.
//
// The generic constraint accepts any string-derived type — WorkerKind,
// TransportKind, tool.Name — so a single helper covers every "what are
// the valid values?" formatting site without each domain type needing
// its own bespoke list-formatter.
func QuotedList[T ~string](vals []T) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Quote(string(v))
	}
	return strings.Join(parts, ", ")
}
