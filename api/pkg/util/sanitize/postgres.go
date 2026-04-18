package sanitize

import (
	"encoding/json"
	"strings"
)

// ForPostgres removes characters that PostgreSQL cannot store in text/jsonb columns.
// PostgreSQL rejects:
//   - \u0000 (null byte) — SQLSTATE 22P05: unsupported Unicode escape sequence
//   - U+D800–U+DFFF (UTF-16 surrogates) — invalid in UTF-8, rejected by jsonb parser
//   - U+FFFE, U+FFFF (non-characters) — some PostgreSQL builds reject these
//
// We also strip other C0 control characters (U+0001–U+0008, U+000B, U+000C,
// U+000E–U+001F) and the C1 range (U+0080–U+009F) which are rarely intentional
// in text content and can cause display issues. We preserve \t (0x09), \n (0x0A),
// and \r (0x0D) as they're legitimate whitespace.
func ForPostgres(s string) string {
	// Fast path: scan for any character that needs removal.
	needsSanitize := false
	for _, r := range s {
		if r == 0 ||
			(r >= 0x01 && r <= 0x08) ||
			r == 0x0B || r == 0x0C ||
			(r >= 0x0E && r <= 0x1F) ||
			(r >= 0x80 && r <= 0x9F) ||
			(r >= 0xD800 && r <= 0xDFFF) ||
			r == 0xFFFE || r == 0xFFFF {
			needsSanitize = true
			break
		}
	}
	if !needsSanitize {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == 0 ||
			(r >= 0x01 && r <= 0x08) ||
			r == 0x0B || r == 0x0C ||
			(r >= 0x0E && r <= 0x1F) ||
			(r >= 0x80 && r <= 0x9F) ||
			(r >= 0xD800 && r <= 0xDFFF) ||
			r == 0xFFFE || r == 0xFFFF {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// JSONForPostgres sanitizes all string values within a JSON blob ([]byte)
// by applying ForPostgres to each string it finds. If the input is empty or
// not valid JSON, it is returned unchanged. The result is re-encoded as compact JSON.
// Intended for JSONB columns that hold agent/LLM output (e.g. response_entries).
func JSONForPostgres(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw // not valid JSON — leave as-is, let Postgres report the error
	}
	v = sanitizeJSONValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return raw
	}
	return out
}

func sanitizeJSONValue(v any) any {
	switch val := v.(type) {
	case string:
		return ForPostgres(val)
	case []any:
		for i, elem := range val {
			val[i] = sanitizeJSONValue(elem)
		}
		return val
	case map[string]any:
		for k, elem := range val {
			val[k] = sanitizeJSONValue(elem)
		}
		return val
	default:
		return v
	}
}
