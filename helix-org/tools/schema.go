package tools

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/helix-org/domain"
)

// schemaOpts are passed to every jsonschema.For call so all our tool
// schemas agree on how to render the special types we use.
//
// json.RawMessage is []byte under the hood; without an override the
// generator emits "array of integer 0..255", which is technically true
// of any byte slice but useless for an MCP client. We treat it as
// "any JSON value" instead.
//
// String-typed enum domains (WorkerKind, TransportKind) are surfaced as
// JSON Schema `enum` constraints so the LLM sees the valid values in
// the tool's input schema and never has to guess (or read source) — and
// any client doing schema validation rejects bad calls *before* they
// reach the tool.
var schemaOpts = &jsonschema.ForOptions{
	TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {Type: "object"},
		reflect.TypeFor[domain.WorkerKind](): enumSchema(
			domain.WorkerKindValues(),
			"Worker kind: human (a person) or ai (a software agent).",
		),
		reflect.TypeFor[domain.TransportKind](): enumSchema(
			domain.TransportKindValues(),
			"Stream transport: local (in-process), webhook (HTTP), email (Postmark), github (inbound).",
		),
	},
}

// enumSchema builds an "enum-constrained string" schema for a
// string-typed domain enum. Centralising this means new enum domains
// (e.g. a future GrantScope) get the right shape automatically.
func enumSchema[T ~string](vals []T, description string) *jsonschema.Schema {
	out := make([]any, len(vals))
	for i, v := range vals {
		out[i] = string(v)
	}
	return &jsonschema.Schema{
		Type:        "string",
		Enum:        out,
		Description: description,
	}
}

// mustSchema builds a JSON Schema from the given args type T at package
// init time. A failure here is a build-time invariant violation (the
// args type is not representable as JSON Schema), not a runtime data
// problem — panicking is the right response and the test suite catches
// it.
func mustSchema[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](schemaOpts)
	if err != nil {
		panic(fmt.Sprintf("jsonschema.For[%T]: %v", *new(T), err))
	}
	return s
}
