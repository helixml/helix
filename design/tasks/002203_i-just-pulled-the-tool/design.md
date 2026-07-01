# Design: Fix MCP Array Parameters Published as String (create_bot tools/topics)

## Investigation summary (what is and isn't broken)

The Go source of truth is **correct**. `CreateBot.InputSchema()`
(`api/pkg/org/interfaces/mcptools/create_bot.go:51`) overrides `tools` and
`topics` with clean array schemas built by
`enumStringArrayProperty` / `stringArrayProperty`
(`api/pkg/org/interfaces/mcptools/schema.go:57,76`), each producing
`{type:"array", items:{type:"string"}, ...}`. The unit test
`assertArrayProp` (`bulk_tools_test.go`) already asserts this and passes.

The org MCP server serves that schema verbatim: `registerToolForBot`
(`api/pkg/org/interfaces/server/mcp.go:134`) passes `InputSchema:
t.InputSchema()` into the official go-sdk `Server.AddTool`, which (go-sdk
v1.4.0, `server.go:249`) uses a provided `*jsonschema.Schema` directly.

**So the bug is NOT in the Go schema definition.** It is in a
serialization / conversion layer between that schema and the harness — and
the symptom (`type:"string"` with **no `items`**) is the tell-tale
signature of a specific footgun described below.

### Root-cause signature: nullable union → `string` fallback

Go reflection over a `[]string` field emits the **nullable union**
`"type": ["array","null"]` (the schema helpers' own comments call this out:
`schema.go:52-56` — "never receives the `["null","array"]` union the
reflection-based generator would emit for a Go slice").

Downstream schema converters read `type` as a scalar string. The MCP
client bridge does exactly this at
`api/pkg/agent/skill/mcp/mcp_skill.go:198-218`:

```go
if typeVal, ok := data["type"].(string); ok && typeVal != "" {
    switch typeVal { case "array": ... }
} else {
    def.Type = jsonschema.String   // <-- silent fallback
}
```

When `type` arrives as `["array","null"]` (a `[]any`, not a `string`), the
`.(string)` assertion fails and the property collapses to bare
`{type:"string"}` with no `items` — **exactly** what the harness reported.
The clean, overridden `create_bot` schema does NOT carry the union, but any
path that surfaces the **reflected** base schema (`mustSchema[createBotArgs]`
before the `withProperty` override, or a generic reflect-based registration)
would, and it would then flatten to `string` here.

Two possibilities, to be disambiguated by Task 1:
1. The harness receives the **reflected base** schema (union types) rather
   than the overridden `InputSchema()` for these params.
2. The harness receives the correct array schema but a converter in its
   path flattens it (union handling and/or a lossy round-trip).

We must **capture the actual wire bytes before writing the fix** — fixing
blind risks patching the wrong layer.

## Approach

### Step 1 — Reproduce at the wire (mandatory, before any code change)

Capture the schema exactly as the harness sees it. Options, cheapest first:
- Dump `create_bot`'s served JSON from the running inner Helix by hitting
  the per-bot MCP endpoint `tools/list` (`/workers/{id}/mcp`, Streamable
  HTTP), or
- Add a temporary `json.Marshal` of `CreateBot{}.InputSchema()` and of the
  reflected `createBotSchema` base in a throwaway test and diff them.

Confirm which of the two possibilities above is real and pin the exact
layer that emits `type:"string"`.

### Step 2 — Fix the identified layer

Depending on Step 1:

- **If the reflected union leaks into what's served** — ensure the served
  schema is always the overridden `InputSchema()` (clean array), and/or
  strip the `null` from unioned array types at registration so `type` is the
  scalar `"array"`. (Satisfies AC-3.)

- **Harden the converter regardless** (robustness fix, prevents recurrence
  for every `[]string` param): in `convertMapToDefinition`
  (`mcp_skill.go:194`), when `data["type"]` is a `[]any` union, pick the
  non-`null` member (`array`/`string`/…) instead of falling through to the
  `string` default. Continue to honour `items`. This is the general fix and
  should be applied even if Step 1 points at the server side, because the
  silent `string` fallback is a latent bug for any nullable field.

Prefer a fix at the layer that actually emits the bad bytes; add the
converter hardening as defence-in-depth.

### Step 3 — Wire-level regression test (TDD red → green)

This is driven test-first. **Write the wire-level test BEFORE the fix and
watch it fail (RED)**, reproducing the reported `type:"string"` symptom, so
we prove the test actually exercises the broken path. The test asserts the
schema **as a client receives it** (serialize to JSON and re-parse, or drive
`tools/list`), checking for each of the five params: `type == "array"` and
`items.type == "string"`, and that `type` is never `string` nor a union.
Only then apply Step 2 and confirm the test passes (GREEN). This ordering
closes the gap the passing `assertArrayProp` left open — that Go-level check
was green throughout the live bug, so a wire-level RED test is what proves
the reproduction.

## Key decisions

- **Fix on the server/contract side, not the harness.** The harness is
  correct to obey `type`; the schema is what lies.
- **Cover all five bulk tools**, not just `create_bot`, since they share the
  helpers and the same failure mode.
- **Test the wire, not the in-Go struct.** The existing Go-level assertion
  passed throughout the bug; only a serialized/observed check would have
  caught it.

## Affected files (candidates — confirm in Step 1)

- `api/pkg/org/interfaces/mcptools/create_bot.go` — `InputSchema()` override.
- `api/pkg/org/interfaces/mcptools/schema.go` — array-property helpers;
  possible union-stripping.
- `api/pkg/org/interfaces/server/mcp.go` — `registerToolForBot` / go-sdk
  registration (what is actually served).
- `api/pkg/agent/skill/mcp/mcp_skill.go` — `convertMapToDefinition` union
  handling / `string` fallback (converter hardening + defence-in-depth).
- `api/pkg/org/interfaces/mcptools/bulk_tools_test.go` — extend with the
  wire-level assertion.
