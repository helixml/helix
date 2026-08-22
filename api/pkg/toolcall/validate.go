// Package toolcall validates the tool calls a model returned against the tool
// schemas the caller supplied.
//
// This measures structural validity — did the model emit something the caller
// could actually dispatch — not whether the tool then succeeded. A call fails
// in exactly one of three ways, in priority order: its arguments are not JSON,
// it names a tool that was never offered, or its arguments do not satisfy that
// tool's parameter schema. The buckets mirror OpenRouter's tool call error rate
// so our per-provider numbers are comparable with theirs.
package toolcall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

// Error kinds. Stored comma-joined on the LLM call so a bad call can be
// triaged without re-reading the request and response bodies.
const (
	KindInvalidJSON    = "invalid_json"
	KindUnknownName    = "unknown_name"
	KindSchemaMismatch = "schema_mismatch"
)

// Tool is a tool offered to the model, normalised across providers. Schema is
// the JSON Schema for the arguments (OpenAI `function.parameters`, Anthropic
// `input_schema`). An absent or uncompilable schema means the tool is treated
// as always valid — we are measuring the model, not the caller's schema.
type Tool struct {
	Name   string
	Schema json.RawMessage
}

// Call is one tool call the model returned. Arguments is the raw argument
// payload as the model emitted it: a JSON string for OpenAI-shaped providers,
// and the re-encoded object for providers that hand back decoded input
// (Anthropic tool_use), where the invalid-JSON bucket cannot fire.
type Call struct {
	Name      string
	Arguments string
}

// Result is the per-request verdict.
type Result struct {
	ToolsOffered int
	Calls        int
	Errors       int
	// Kinds are the distinct failure buckets seen in this request, sorted.
	Kinds []string
}

// KindsString renders Kinds for storage on the LLM call row.
func (r Result) KindsString() string { return strings.Join(r.Kinds, ",") }

// Errored reports whether the request counts against the tool call error rate.
// A request is errored if any of its calls failed, matching OpenRouter's
// request-level aggregation: one bad call out of five is one errored request,
// not 20% of one.
func (r Result) Errored() bool { return r.Errors > 0 }

// Validate checks every call against the offered tools.
func Validate(tools []Tool, calls []Call) Result {
	result := Result{ToolsOffered: len(tools), Calls: len(calls)}
	if len(calls) == 0 {
		return result
	}

	schemas := make(map[string]json.RawMessage, len(tools))
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		schemas[tool.Name] = tool.Schema
	}

	kinds := make(map[string]bool, 3)
	for _, call := range calls {
		kind := validateCall(schemas, call)
		if kind == "" {
			continue
		}
		result.Errors++
		kinds[kind] = true
	}

	for kind := range kinds {
		result.Kinds = append(result.Kinds, kind)
	}
	sort.Strings(result.Kinds)
	return result
}

// validateCall returns the failure bucket for one call, or "" if it is valid.
// A call that fails more than one way is charged to the first bucket that
// applies, so the counts stay one-per-call.
func validateCall(schemas map[string]json.RawMessage, call Call) string {
	arguments := strings.TrimSpace(call.Arguments)
	if arguments == "" {
		// A no-argument tool call is commonly emitted as an empty string
		// rather than "{}". Reading it as the empty object keeps that off the
		// invalid-JSON pile; if the tool actually required arguments, the
		// schema check below still catches it as a mismatch.
		arguments = "{}"
	}

	var decoded any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return KindInvalidJSON
	}

	schema, offered := schemas[call.Name]
	if !offered {
		return KindUnknownName
	}

	resolved := compile(schema)
	if resolved == nil {
		return ""
	}
	if err := resolved.Validate(decoded); err != nil {
		return KindSchemaMismatch
	}
	return ""
}

// Coding agents resend the same handful of tool schemas on every turn, so
// compiling per call would dominate the cost of logging. Cache by schema
// content; the map is cleared wholesale once it grows past the bound rather
// than evicting entry by entry, which is enough for a working set this small.
const maxCachedSchemas = 1024

var (
	schemaCacheMu sync.RWMutex
	schemaCache   = map[string]*jsonschema.Resolved{}
)

// compile returns the resolved schema, or nil when the tool should be treated
// as always valid (no schema, or a schema we cannot compile).
func compile(raw json.RawMessage) *jsonschema.Resolved {
	if len(raw) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	sum := sha256.Sum256([]byte(trimmed))
	key := hex.EncodeToString(sum[:])

	schemaCacheMu.RLock()
	resolved, cached := schemaCache[key]
	schemaCacheMu.RUnlock()
	if cached {
		return resolved
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal([]byte(trimmed), &schema); err == nil {
		if compiled, err := schema.Resolve(nil); err == nil {
			resolved = compiled
		}
	}

	schemaCacheMu.Lock()
	if len(schemaCache) >= maxCachedSchemas {
		schemaCache = map[string]*jsonschema.Resolved{}
	}
	schemaCache[key] = resolved
	schemaCacheMu.Unlock()

	return resolved
}
