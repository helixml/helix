package mcptools

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// schemaOpts are passed to every jsonschema.For call so all our tool
// schemas agree on how to render the special types we use.
//
// json.RawMessage is []byte under the hood; without an override the
// generator emits "array of integer 0..255", which is technically true
// of any byte slice but useless for an MCP client. We treat it as
// "any JSON value" instead.
//
// String-typed enum domains (TransportKind, processor.Kind) are surfaced
// as JSON Schema `enum` constraints so the LLM sees the valid values in
// the tool's input schema and never has to guess (or read source) — and
// any client doing schema validation rejects bad calls *before* they
// reach the tool.
var schemaOpts = &jsonschema.ForOptions{
	TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {Type: "object"},
		reflect.TypeFor[transport.Kind](): enumSchema(
			transport.KindValues(),
			"Topic transport: local (in-process), webhook (HTTP), email (Postmark), github (inbound).",
		),
		reflect.TypeFor[processor.Kind](): enumSchema(
			processor.KindValues(),
			`Processor kind: "template", "truncate", "filter", or "js".`,
		),
	},
}

// enumSchema builds an "enum-constrained string" schema for a
// string-typed domain enum. Centralising this means new enum domains
// (e.g. a future transport kind) get the right shape automatically.
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

// enumStringArrayProperty builds a non-nullable array property whose
// items are constrained to the given tool-name enum. Used for the
// `tools` argument on create_bot/attach_tool/detach_tool so the LLM sees
// exactly the valid tool names and never receives the `["null","array"]`
// union the reflection-based generator would emit for a Go slice.
func enumStringArrayProperty(names []tool.Name, description string) *jsonschema.Schema {
	item := &jsonschema.Schema{Type: "string"}
	if len(names) > 0 {
		enum := make([]any, len(names))
		for i, n := range names {
			enum[i] = string(n)
		}
		item.Enum = enum
	}
	return &jsonschema.Schema{
		Type:        "array",
		Description: description,
		Items:       item,
	}
}

// stringArrayProperty builds a non-nullable array-of-strings property
// (no enum). Used for dynamic-valued lists like topic ids, so the schema
// is a plain array rather than the `["null","array"]` union.
func stringArrayProperty(description string) *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:        "array",
		Description: description,
		Items:       &jsonschema.Schema{Type: "string"},
	}
}

// withProperty returns a shallow copy of base with one property replaced
// (the Properties map is cloned so the shared base schema is never
// mutated — InputSchema() may run concurrently). Lets a tool start from
// its reflected base schema (which carries required + additionalProperties)
// and swap in a dynamically-built property.
func withProperty(base *jsonschema.Schema, name string, prop *jsonschema.Schema) *jsonschema.Schema {
	s := *base
	props := make(map[string]*jsonschema.Schema, len(base.Properties))
	for k, v := range base.Properties {
		props[k] = v
	}
	props[name] = prop
	s.Properties = props
	return &s
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
