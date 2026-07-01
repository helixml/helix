# Implementation Tasks: Fix External MCP Proxy Flattening Array Params to String (create_bot/subscribe)

Root cause is confirmed: `ExternalMCPBackend` re-advertises every proxied tool
property as `mcp.WithString`, flattening arrays. Fix = forward the upstream
schema verbatim. Follow TDD.

## RED — reproduce the flattening (against REAL production code)

- [x] **DONE — proper RED, proven this session.** Extracted the per-tool schema
      reconstruction out of `getOrCreateServer` into a production helper
      `buildProxyTool(mcp.Tool) mcp.Tool` (behavior-preserving; `getOrCreateServer`
      now calls it), then wrote `TestBuildProxyTool_PreservesArrayParams` that
      calls **that real function** with an upstream `create_bot` whose `tools` is
      `type:"array", items:{type:"string"}`.
      - Against current code: **FAILS** — `create_bot.tools re-advertised as
        "string", want "array"` (the exact blocker).
      - After the passthrough fix: **PASSES**; `content` stays `string`; build clean.
      Proof + commands: `red_test/PROOF.md`. Turnkey artifacts:
      `red_test/mcp_backend_external_schema_test.go` and
      `red_test/production_fix.patch`. Needs `CGO_ENABLED=1` (tree-sitter).
      Kept out of the helix repo until the plan is approved.

## GREEN — verbatim schema passthrough

- [~] Apply `red_test/production_fix.patch` (or hand-apply): `buildProxyTool`
      forwards the upstream schema verbatim via
      `mcp.NewToolWithRawSchema(tool.Name, tool.Description, json.Marshal(tool.InputSchema))`;
      on marshal error, log and serve a description-only tool (never fall back to
      an all-string schema). Add the `encoding/json` import.
- [ ] Also add the test file `red_test/mcp_backend_external_schema_test.go` to
      `api/pkg/server/`. Run it; confirm GREEN (proven this session).

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
