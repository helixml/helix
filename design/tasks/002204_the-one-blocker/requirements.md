# Requirements: Roll Out and Verify create_bot/subscribe MCP Array-Parameter Fix

## Problem

A Bot in the org runtime re-checked the live `create_bot` tool schema first
thing (standing protocol) and reports it *still* advertises the array
parameters as scalars:

```json
"tools":  { "type": "string", ... }
"topics": { "type": "string", ... }
```

Because the machine-readable `type` says `string`, a spec-compliant harness
serialises `["a","b"]` to the string `"[\"a\",\"b\"]"`. The Go handler
(`createBotArgs.Tools []string` / `Topics []string`) then rejects it with
`cannot unmarshal string into []string`, so `create_bot` and `subscribe`
keep failing. The Bot correctly did **not** retry (identical error) and asked
to be re-activated fresh once the runtime advertises `type:"array",
items:string`.

## Critical finding (investigation)

**The code fix is already merged and sits at HEAD of `main`.**

- Commit `169cb6421` — "Fix MCP array parameters published as string
  (create_bot tools/topics) (#2774)" — is the current `main` HEAD.
- The org MCP **server** already publishes clean `type:"array", items:string`
  for `create_bot.tools/topics` (and `attach_tool`, `detach_tool`,
  `subscribe.topicIds`, `unsubscribe.topicIds`). This landed in #2768 via the
  `InputSchema()` overrides in `mcptools/schema.go`, and is locked by the
  wire-level regression test `TestSchemaWireArrayParams`
  (`api/pkg/org/interfaces/mcptools/schema_wire_test.go`).
- The Go MCP **consumer** (`convertMapToDefinition` in
  `api/pkg/agent/skill/mcp/mcp_skill.go`) had a bug where a nullable-union
  `type` (`["array","null"]`) fell back to `string`. #2774 added
  `resolveSchemaType` to resolve the non-null member. Also fixed.

The prior task (002203) that produced #2774 explicitly recorded that it was
verified **in-process only** — "NOT tested against a live LLM harness in the
inner Helix stack." **That untested live gap is exactly this blocker.**

Therefore this is **not a new code change**. The Bot sees `string` because
the runtime it talks to is running a build that predates #2768/#2774, and/or
its activation cached the old tool list — matching the Bot's own request to
be re-activated after the runtime picks up the fix.

## User Stories

- **As the reporting Bot**, once the runtime advertises
  `tools`/`topics` as `type:"array", items:string`, and I am woken in a fresh
  activation, I want `create_bot` and `subscribe` to succeed when I pass real
  JSON arrays — so I can create tooled-up, subscribed Bots in one call.
- **As a maintainer**, I want the fix confirmed against the *live* stack (not
  just in-process), closing the verification gap task 002203 left open.

## Acceptance Criteria

1. The runtime the Bots connect to is running a build that includes #2768 and
   #2774 (verify the deployed commit, not just that `main` has it).
2. A live `tools/list` against the org MCP endpoint shows, for `create_bot`,
   `attach_tool`, `detach_tool`, `subscribe`, `unsubscribe`, that every array
   parameter is `type:"array"` with `items.type:"string"` — never `"string"`
   and never a `["array","null"]` union.
3. A live `create_bot` call passing `"tools": [...]` and `"topics": [...]` as
   real JSON arrays **succeeds** (no `cannot unmarshal string into []string`).
4. A live `subscribe` call passing `"topicIds": [...]` as a JSON array
   **succeeds**.
5. The reporting Bot is re-activated in a fresh activation so it re-reads the
   corrected schema.
6. **Contingency:** if, after redeploy + fresh activation, the live schema or
   an LLM-visible tool definition still shows `string`, a consumer conversion
   path was missed (e.g. the desktop harness's own MCP client). That path is
   identified and fixed, and criteria 2–4 re-verified.

## Out of Scope

- Re-implementing the already-merged schema/consumer fix.
- Changing the `create_bot`/`subscribe` handler contract or arguments.
