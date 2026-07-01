# Implementation Tasks: Fix MCP Array Parameters Published as String (create_bot tools/topics)

Follow strict TDD: write a failing (RED) test that replicates the reported
bug, watch it fail for the right reason, then make it GREEN with the minimal
fix. Do not write the fix before the test fails.

## Investigation result (updated during implementation)

Empirically pinned the layer (see design.md "Investigation result"):
- The org MCP server **already publishes clean arrays** for all five params
  (`type:"array"` + `items`), verified via a full go-sdk `tools/list`
  round-trip AND after mark3labs client parsing. So the server is NOT the
  defect at HEAD.
- The defect is in the **MCP client-consumption converter**
  `convertMapToDefinition` (`api/pkg/agent/skill/mcp/mcp_skill.go`): when a
  param's `type` is the nullable **union** `["array","null"]` (the form
  reflection-based schema generators emit for slice/pointer fields), the
  `data["type"].(string)` assertion fails and it silently falls back to
  `jsonschema.String` — reproducing the reported `type:"string"` symptom.

## RED — replicate the bug with a failing test

- [x] Reproduce and pin the exact emitting layer (done — it is
  `convertMapToDefinition`'s union → `string` fallback, not the org server).
- [x] Write the RED test (`api/pkg/agent/skill/mcp`): feed a create_bot-like
  schema whose array params use the union form `["array","null"]` through
  `buildParameters` and assert `type == "array"`. Confirmed it FAILS with
  `got "string"` — the reported symptom.

## GREEN — minimal fix until the test passes

- [x] Harden `convertMapToDefinition` to resolve a `[]any` union `type` to
  its non-`null` member (array/string/…) instead of defaulting to `string`.
  Fixes every `[]string`/nullable param, not just create_bot.
- [x] Run the RED test again; confirm GREEN.
- [~] Add a wire-level guard test in `api/pkg/org/interfaces/mcptools` that
  serializes the served schema (go-sdk `tools/list` round-trip) and asserts
  all five params (`create_bot.tools`, `create_bot.topics`,
  `attach_tool.tools`, `detach_tool.tools`, `subscribe.topicIds`,
  `unsubscribe.topicIds`) publish `type:"array"` + `items.type:"string"` and
  never `string`/a union — so the server side can't regress into emitting the
  union that the converter used to choke on. Keep existing `assertArrayProp`.

## Verify

- [ ] `go build ./...` for touched packages; `go test` for
  `api/pkg/org/interfaces/mcptools/` and `api/pkg/agent/skill/mcp/`.
- [ ] Check CI green after pushing.
