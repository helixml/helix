# Implementation Tasks: Fix MCP Array Parameters Published as String (create_bot tools/topics)

Follow strict TDD: write a failing (RED) test that replicates the reported
bug at the wire, watch it fail for the right reason, then make it GREEN with
the minimal fix. Do not write the fix before the test fails.

## RED — replicate the bug with a failing test

- [ ] Reproduce at the wire first: capture the `create_bot` schema exactly as a harness receives it (drive `tools/list` on `/workers/{id}/mcp`, or `json.Marshal` both `InputSchema()` and the reflected `createBotSchema` base and diff). Confirm the observed `type:"string"` (no `items`) and pin the exact emitting layer (leaked reflected nullable-union vs `convertMapToDefinition` `type.(string)` fallback vs a lossy round-trip).
- [ ] Write the RED test: a wire-level test that serializes/observes the schema the way a client does and asserts, for `create_bot.tools` and `create_bot.topics`, `type == "array"` + `items.type == "string"` (and `type` is never `string` nor a union). Run it and confirm it FAILS, reproducing the reported symptom — not failing for an unrelated reason (wrong endpoint, nil map, etc.).
- [ ] Extend the RED test to the other four params so they fail too if affected: `attach_tool.tools`, `detach_tool.tools`, `subscribe.topicIds`, `unsubscribe.topicIds`.

## GREEN — minimal fix until the test passes

- [ ] Fix the emitting layer so `tools`/`topics` publish `{type:"array", items:{type:"string"}}` with a scalar `type` (never `["array","null"]`); preserve the `tools` enum of valid tool names.
- [ ] Harden `convertMapToDefinition` (`api/pkg/agent/skill/mcp/mcp_skill.go`) to resolve a `[]any` union `type` to its non-`null` member instead of silently defaulting to `string` (defence-in-depth for all `[]string` params).
- [ ] Run the RED test again and confirm it now passes (GREEN) for all five params. Keep the existing `assertArrayProp` Go-level check alongside the new wire-level test.

## Verify

- [ ] Verify end-to-end against the inner Helix: a harness call to `create_bot` with `tools: ["publish","subscribe"]`, `topics: [...]` succeeds, and with `tools: []`, `topics: []` succeeds.
- [ ] `go build ./...` for the touched packages; run `go test` for `api/pkg/org/interfaces/mcptools/` and the agent mcp skill; check CI green after pushing.
