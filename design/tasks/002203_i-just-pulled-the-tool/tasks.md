# Implementation Tasks: Fix MCP Array Parameters Published as String (create_bot tools/topics)

- [ ] Reproduce at the wire: capture the `create_bot` schema exactly as a harness receives it (drive `tools/list` on `/workers/{id}/mcp`, or `json.Marshal` both `InputSchema()` and the reflected `createBotSchema` base and diff). Confirm whether `type:"string"` comes from a leaked reflected nullable-union or from a converter flattening a correct array.
- [ ] Pin the exact layer that emits `type:"string"` (server registration vs `convertMapToDefinition` `type.(string)` fallback vs a lossy round-trip) before changing code.
- [ ] Fix the emitting layer so `create_bot.tools` and `create_bot.topics` are published as `{type:"array", items:{type:"string"}}` with a scalar `type` (never `["array","null"]`); preserve the `tools` enum of valid tool names.
- [ ] Harden `convertMapToDefinition` (`api/pkg/agent/skill/mcp/mcp_skill.go`) to resolve a `[]any` union `type` to its non-`null` member instead of silently defaulting to `string` (defence-in-depth for all `[]string` params).
- [ ] Apply/verify the same correct publishing for the other four bulk tools: `attach_tool.tools`, `detach_tool.tools`, `subscribe.topicIds`, `unsubscribe.topicIds`.
- [ ] Add a wire-level regression test (serialize/observe the schema as a client would) asserting `type == "array"` + `items.type == "string"` for all five params, and that `type` is never `string` nor a union. Keep the existing `assertArrayProp` Go-level check.
- [ ] Verify end-to-end against the inner Helix: a harness call to `create_bot` with `tools: ["publish","subscribe"]`, `topics: [...]` succeeds, and with `tools: []`, `topics: []` succeeds.
- [ ] `go build ./...` for the touched packages; run `go test` for `api/pkg/org/interfaces/mcptools/` and the agent mcp skill; check CI green after pushing.
