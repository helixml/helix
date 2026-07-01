# Fix MCP array parameters published as string (create_bot tools/topics)

## Summary

A harness reported that `create_bot`'s `tools` and `topics` parameters were
published with `"type": "string"` instead of `"array"`. Because a
spec-honouring harness serializes whatever it puts in a `string` slot **to a
string**, arrays went on the wire as `"[\"a\",\"b\"]"` and the Go handler
(which wants `[]string`) rejected them — making the tool uncallable.

Root-causing (reproduced in-process across the full pipeline) showed the org
MCP server is **already correct**: it publishes `{type:"array",
items:{type:"string"}}` for all five bulk-tool array params, verified via a
go-sdk `tools/list` round-trip and after mark3labs client parsing.

The actual defect is in the **MCP client-consumption converter**
`convertMapToDefinition` (`api/pkg/agent/skill/mcp/mcp_skill.go`). When a
parameter's `type` arrives as the JSON Schema nullable **union**
`["array","null"]` — the shape reflection-based schema generators emit for Go
slice/pointer fields — the old `data["type"].(string)` type assertion failed
(the value is a `[]any`, not a string) and silently fell back to
`jsonschema.String`, collapsing the array to a scalar string. This is the
exact `type:"string"` symptom, and it affected *every* nullable/array MCP
parameter, not just `create_bot`.

## Changes

- `fix(agent)`: add `resolveSchemaType` and use it in `convertMapToDefinition`
  so a `type` that is a scalar string **or** a nullable union (`["array","null"]`,
  `["null","array"]`, …) resolves to its non-`null` member. Array params now
  survive as arrays instead of falling back to `string`.
- `test(agent)`: RED→GREEN regression tests — a create_bot-like schema with
  union-typed array params through `buildParameters`, plus a table covering
  union ordering, plain scalars, and missing type.
- `test(org)`: wire-level guard (`schema_wire_test.go`) that serves all five
  bulk tools over a go-sdk `tools/list` round-trip and asserts
  `create_bot.tools/topics`, `attach_tool.tools`, `detach_tool.tools`,
  `subscribe.topicIds`, `unsubscribe.topicIds` publish `type:"array"` +
  `items.type:"string"` — never `string` and never a union. This catches
  what the existing in-Go `assertArrayProp` check could not (it stayed green
  throughout the live bug because it inspects the schema struct, not the wire).

## Testing

- `go build ./pkg/agent/... ./pkg/org/...` — passes.
- `go test ./pkg/agent/skill/mcp/ ./pkg/org/interfaces/mcptools/` — green.
- Verified across the real consumption path reproduced in-process (go-sdk
  `tools/list` → mark3labs parse → `buildParameters`). Not exercised against a
  live LLM harness in the inner stack; the reproduced path is the code that
  produces the harness-visible schema.
