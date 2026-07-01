# Requirements: Fix MCP Array Parameters Published as String (create_bot tools/topics)

## Problem

A harness pulled the `create_bot` tool definition fresh from the helix MCP
server and the two array parameters are published with `"type": "string"`:

```json
"tools":  { "type": "string", "description": "MCP tools to grant … pass [] for the read baseline only" },
"topics": { "type": "string", "description": "Existing Topic ids … pass [] for none" }
```

The prose says "array", but the machine-readable `type` says `string`. A
spec-compliant harness serializes whatever it puts in a `string` slot **to a
string** — so `["hi","hello"]` goes on the wire as `"[\"hi\",\"hello\"]"`.
The Go handler (`createBotArgs.Tools []string` / `Topics []string`) then
rejects that string because it wants a JSON array. Result: `create_bot`
(and the other bulk tools) are uncallable by any harness that honours the
declared `type`.

This is a schema-vs-handler contract violation, not a harness bug. The fix
must make the **published** schema declare `array` so the type the harness
sees matches the type the handler unmarshals.

## Scope

The same array-property helpers back five tools. All must publish `array`:

| Tool | Array params |
|------|--------------|
| `create_bot` | `tools`, `topics` |
| `attach_tool` | `tools` |
| `detach_tool` | `tools` |
| `subscribe` | `topicIds` |
| `unsubscribe` | `topicIds` |

## User Stories

**US-1 — As an agent/harness calling `create_bot`, the tool schema must
declare `tools` and `topics` as `array` of `string`**, so my harness sends a
real JSON array and the call succeeds.

- AC-1: The schema observed by the harness for `create_bot.tools` and
  `create_bot.topics` has `"type": "array"` with `"items": {"type": "string"}`.
- AC-2: `tools` additionally carries the `enum` of valid tool names (its
  existing behaviour must survive the fix).
- AC-3: `type` is the single string `"array"`, NOT the nullable union
  `["array","null"]` (harnesses that expect a scalar `type` must not
  silently fall back to `string`).
- AC-4: Passing `["publish","subscribe"]` and `[]` both succeed end-to-end
  (create a bot with tools+topics, and with empty arrays).

**US-2 — As a maintainer, the same fix must cover all five bulk tools**, so
we don't reopen this bug via `attach_tool` / `subscribe` / etc.

- AC-5: `attach_tool.tools`, `detach_tool.tools`, `subscribe.topicIds`,
  `unsubscribe.topicIds` all publish `array`/`items` in the observed schema.

**US-3 — As a maintainer, a regression test must assert the WIRE schema**,
not just the in-Go `*jsonschema.Schema`, so the "correct in Go, wrong on the
wire" gap that caused this cannot recur silently.

- AC-6: A test serializes/observes the schema the way a client receives it
  and asserts `type == "array"` + `items.type == "string"` for the params
  above. The existing `assertArrayProp` Go-level check is necessary but not
  sufficient (it passed while the bug was live).

## Out of Scope

- Redesigning the org MCP tool surface or the `create_bot` argument set.
- Changing harness behaviour — the contract is fixed on the server side.
