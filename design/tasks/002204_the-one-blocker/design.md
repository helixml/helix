# Design: Roll Out and Verify create_bot/subscribe MCP Array-Parameter Fix

## Summary

The `cannot unmarshal string into []string` blocker is a **rollout +
live-verification** problem, not a fresh code defect. The fix is already in
`main` (HEAD = `169cb6421`, #2774; server side since #2768). The live runtime
the Bot talks to simply hasn't picked it up. This design covers confirming
the deployed build, verifying end-to-end against the live stack, re-activating
the Bot, and a contingency for a missed consumer path.

## Two layers, both already fixed at HEAD

```
create_bot args (Go struct)              LLM-visible tool schema
  Tools []string / Topics []string             ▲
        │ reflection → ["array","null"]         │ buildParameters /
        ▼                                        │ convertMapToDefinition
  mcptools/schema.go  ── InputSchema() ──▶  MCP wire schema  ──▶  agent MCP client
   (PRODUCER)          overrides tools/topics   type:"array"        (CONSUMER)
                       to clean type:"array"     items:string
```

- **Producer** (`api/pkg/org/interfaces/mcptools/schema.go`): `InputSchema()`
  swaps the reflected nullable-union properties for clean arrays
  (`enumStringArrayProperty` / `stringArrayProperty`). Guarded by
  `TestSchemaWireArrayParams` (a real go-sdk `tools/list` round-trip).
- **Consumer** (`api/pkg/agent/skill/mcp/mcp_skill.go`): `resolveSchemaType`
  now resolves `["array","null"]` → `array` instead of falling back to
  `string`. Guarded by `buildparameters_union_test.go`.

Both are at HEAD. Nothing in source needs re-writing.

## Why the live Bot still sees `string`

The Bot connects to a *running* org MCP server and a *running* agent harness.
Either or both predate the fix:

- **Stale API build.** In dev, Air hot-reloads Go, so `helix-api-1` at HEAD
  serves the corrected schema. But if the Bot talks to a differently-versioned
  API (outer stack, an un-rebuilt prod, or an API container that hasn't
  rebuilt since before #2768), it serves the old reflected `string`.
- **Stale desktop/agent image.** Org Bots run in desktop sandboxes
  (`helix-ubuntu:<sha>`, Qwen Code + Zed), which are **version-pinned and not
  hot-reloaded**. If the Bot's tool schema is produced by the desktop
  harness's own MCP client, that image must include the consumer fix.
- **Cached activation.** The Bot's current activation captured the old
  tool list at boot; it needs a fresh activation to re-read `tools/list` —
  exactly what the Bot asked for.

## Key decisions

1. **Do not touch application code by default.** The fix exists; adding more
   code would be a redundant no-op and risks divergence from the guarded
   behaviour. The primary deliverable is deploy + verify.
2. **Verify the *deployed commit*, not the repo.** `git log` showing the fix
   on `main` is not evidence the runtime runs it. Check the actual container's
   version / rebuild it.
3. **Verify at the wire and at the call**, not just the schema. Assert the
   served `tools/list` types AND make real `create_bot`/`subscribe` calls with
   array args succeed — this is the live check 002203 skipped.
4. **Fresh activation is part of the fix**, per the Bot's protocol. A verified
   schema with a stale activation does not unblock the Bot.

## Verification approach

- **Schema (wire):** run the existing Go tests as a sanity check at HEAD
  (`go test ./api/pkg/org/interfaces/mcptools/... ./api/pkg/agent/skill/mcp/...`),
  then hit the live org MCP `tools/list` endpoint and inspect the raw bytes for
  the five tools' array params.
- **Call (end-to-end):** against the live inner Helix stack, issue a
  `tools/call` for `create_bot` with `"tools":[...]`, `"topics":[]` and a real
  handle, and a `subscribe` with `"topicIds":[...]`. Both must return success,
  not the unmarshal error. This is the faithful live test.
- **Bot:** trigger a fresh activation of the reporting Bot and confirm it
  proceeds past the schema check.

## Contingency: a missed consumer path

If the served schema is `array` but the Bot/LLM still receives `string`, the
desktop harness's MCP-schema conversion is the culprit (the Go
`mcp_skill.go` fix does not reach a TS/Qwen client). Then:

- Locate the desktop harness's MCP inputSchema → tool-declaration converter
  (e.g. `qwen-code/packages/core/src/tools/mcp-client.ts`).
- Check whether it collapses `type:"array"`/nullable unions to string.
- If so, apply the same `resolveSchemaType`-style non-null resolution there,
  rebuild the desktop/qwen image, bump `sandbox-versions.txt`, and re-verify.

Only enter this branch if live verification proves the served schema is
correct yet the consumer still degrades it — do not pre-emptively edit Qwen.

## Notes for future agents

- The producer emits **clean array** since #2768 — a live `string` almost
  always means a stale deployment, not a source bug. Check the deployed build
  first.
- The regression is guarded on both sides; treat a red `TestSchemaWireArrayParams`
  or `buildparameters_union_test` as the real signal that source regressed.
- "Verified in-process" ≠ "works live." That gap is what produced this repeat
  report; close it with an actual `tools/call`.
