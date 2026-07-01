# Implementation Tasks: Fix External MCP Proxy Flattening Array Params to String (create_bot/subscribe)

Root cause is confirmed: `ExternalMCPBackend` re-advertises every proxied tool
property as `mcp.WithString`, flattening arrays. Fix = forward the upstream
schema verbatim. Follow TDD.

## RED — reproduce the flattening

- [ ] Add a test in `api/pkg/server` that feeds a synthetic upstream tool with a
      param `type:"array", items:{type:"string"}` (and one required scalar)
      through the proxy tool-registration logic, marshals the re-served tool
      schema, and asserts the array param serialises as `type:"array"` with
      `items.type:"string"`. Confirm it FAILS against the current
      `WithString`-only code with the reported `type:"string"` symptom.

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
