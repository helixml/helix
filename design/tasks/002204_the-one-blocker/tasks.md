# Implementation Tasks: Fix External MCP Proxy Flattening Array Params to String (create_bot/subscribe)

Root cause is confirmed: `ExternalMCPBackend` re-advertises every proxied tool
property as `mcp.WithString`, flattening arrays. Fix = forward the upstream
schema verbatim. Follow TDD.

## RED — reproduce the flattening

- [x] **DONE (proven this session).** Wrote a test that replicates the proxy's
      verbatim tool reconstruction (`mcp_backend_external.go` L322–358) against
      the real `mark3labs/mcp-go` library, feeding an upstream `create_bot` with
      `tools` as `type:"array", items:{type:"string"}`. Ran it:
      - `TestProxyReadvertise_Current` **FAILS** —
        `create_bot.tools re-advertised as "string", want "array"` (the exact blocker).
      - `TestProxyReadvertise_Fixed` **PASSES** — `NewToolWithRawSchema`
        passthrough keeps `tools` an array and `content` a string.
      Test artifact: `red_test/mcp_backend_external_schema_red_test.go.txt`
      (needs `CGO_ENABLED=1` — tree-sitter). Kept out of the helix repo during
      planning; drop it into `api/pkg/server` in the implementation phase.
- [ ] During implementation, promote it to a genuine regression test that
      exercises the **production** registration (extract the per-tool schema
      build in `getOrCreateServer` into a small pure helper and test that),
      so a future edit to the proxy can't regress while the replica stays green.

## GREEN — verbatim schema passthrough

- [ ] In `api/pkg/server/mcp_backend_external.go` (~L322–358), delete the
      per-property `WithString` loop and register each tool via
      `mcp.NewToolWithRawSchema(tool.Name, tool.Description, raw)` where
      `raw, err := json.Marshal(tool.InputSchema)`.
- [ ] On marshal error, log and skip the tool (do not fall back to string).
- [ ] Run the RED test; confirm GREEN.

## Verify

- [ ] `go build ./...` clean; existing external-MCP tests pass.
- [ ] Sanity: org server still green — `go test ./api/pkg/org/interfaces/mcptools/ -run TestSchemaWireArrayParams`.
- [ ] Live (inner Helix): in a `zed_external` session with an HTTP MCP exposing
      an array param, confirm the LLM-visible tool advertises an array; if
      feasible, drive a real `create_bot` with `tools`/`topics` arrays and a
      `subscribe` with `topicIds`, confirming no `cannot unmarshal string into
      []string`. Record the output as evidence.
- [ ] Push and confirm CI green.

## Rollout (post-merge, so the reporting Bot is unblocked)

- [ ] Ensure the runtime the Bots connect to is redeployed with this fix.
- [ ] Re-activate the reporting Bot in a fresh activation so it re-reads the
      corrected schema.
