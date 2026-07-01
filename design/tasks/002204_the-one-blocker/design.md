# Design: Fix External MCP Proxy Flattening Array Params to String (create_bot/subscribe)

## Summary

Fix `ExternalMCPBackend` so it forwards each proxied tool's **full** input
schema to Zed verbatim, instead of rebuilding it as all-strings. This is the
single defect behind the Bot's `cannot unmarshal string into []string`
blocker.

## The path an org Bot's tool schema actually travels

```
org MCP server                     ExternalMCPBackend                 Zed / Qwen
/orgs/{org}/workers/{id}/mcp  ──▶  /api/v1/mcp/external/helix   ──▶   agent → LLM
  go-sdk, emits                     mark3labs server to Zed,           sees the
  type:"array" ✓ (wire test)        mark3labs client to org server     re-served schema
                                    ▲ BUG: rebuilds every prop
                                      as mcp.WithString → type:"string"
```

- `AttachHelixOrgMCP` (`api/pkg/org/infrastructure/runtime/helix/mcp.go`) adds
  the org endpoint as `AssistantMCP{Name:"helix", Transport:"http", URL:
  .../workers/{id}/mcp}` on the Worker's app.
- `zed_config.go` (~L343) routes HTTP MCPs through
  `/api/v1/mcp/external/{name}` — i.e. `ExternalMCPBackend`.
- `ExternalMCPBackend.getOrCreateServer` (`mcp_backend_external.go`) lists the
  org tools (correct arrays) then **re-registers them for Zed dropping all type
  info** (L322–358): every property becomes `mcp.WithString(...)`.

Verified: `TestSchemaWireArrayParams` (org server) passes at HEAD, so the
upstream schema is correct arrays; the loss happens only in the proxy rebuild.
`createToolHandler` (L406) forwards `request.Params.Arguments` unchanged, so
fixing the advertised schema is sufficient — no change needed on the call path.

## The offending code (mcp_backend_external.go, ~L322–358)

```go
for propName, propDef := range tool.InputSchema.Properties {
    // ...only reads description...
    if required[propName] {
        opts = append(opts, mcp.WithString(propName, mcp.Required(), mcp.Description(desc)))
    } else {
        opts = append(opts, mcp.WithString(propName, mcp.Description(desc)))
    }
}
mcpTool := mcp.NewTool(tool.Name, opts...)
```

Every param → string. `type`, `items`, `enum`, nested `properties` all lost.

## Chosen fix: pass the upstream schema through verbatim

mark3labs v0.47.0 exposes `mcp.NewToolWithRawSchema(name, description,
schema json.RawMessage)` and `Tool.RawInputSchema`. Marshal the upstream
`tool.InputSchema` (a `mcp.ToolInputSchema` whose `Properties` map already
holds the fully-decoded `type`/`items`/`enum`) and hand it over unchanged:

```go
raw, err := json.Marshal(tool.InputSchema)
if err != nil {
    // skip this tool (log) rather than degrade it to strings
    continue
}
mcpTool := mcp.NewToolWithRawSchema(tool.Name, tool.Description, raw)
mcpServer.AddTool(mcpTool, handler)
```

Delete the per-property `WithString` loop entirely.

### Why passthrough over per-type mapping

- **Lossless & future-proof.** Preserves arrays, items, enums, nested objects,
  numbers, booleans — for every current and future tool — without a
  type-switch that would need extending per new construct.
- **Faithful to the upstream contract.** The org server already publishes the
  exact JSON Schema it wants clients to see; the proxy's job is transport, not
  reinterpretation. Re-deriving the schema is what introduced the bug.
- The alternative (a `type`-switch adding `WithArray`/`WithObject`/…) is more
  code, still lossy on anything unhandled, and re-introduces the same class of
  bug #2774 already had to patch elsewhere.

### Edge cases

- `tool.InputSchema` marshals to `{"type":"object","properties":{...},
  "required":[...]}` (mark3labs `ToolInputSchema.MarshalJSON`); it is never nil
  for these tools. If a future tool has empty properties, the raw schema is a
  valid empty object — no regression.
- mark3labs errors if both `InputSchema` and `RawInputSchema` are set;
  `NewToolWithRawSchema` sets only the raw form, so no conflict.

## Testing

- **Unit / regression** (`api/pkg/server`): build an `ExternalMCPBackend`-style
  registration from a synthetic upstream tool whose param is
  `type:"array", items:{type:"string"}`, marshal the re-served tool, and assert
  the served param is `type:"array"` with `items.type:"string"` — not
  `"string"`. This fails against the old `WithString` code (RED) and passes
  after the fix (GREEN).
- **Build**: `go build ./...`; run existing external-MCP tests.
- **Live (inner Helix)**: exercise a `zed_external` session whose app has an
  HTTP MCP with an array param and confirm the LLM-visible tool shows an array;
  ideally drive an actual `create_bot` with `tools`/`topics` arrays end-to-end.

## Notes for future agents

- Two MCP consumer paths exist. `zed_external` agents (org Bots, desktop Zed)
  use `ExternalMCPBackend`; the in-process Helix Go agent uses
  `pkg/agent/skill/mcp`. A schema bug can live in either — fix the one the
  failing agent actually uses. #2774 fixed the latter; this fixes the former.
- Rule of thumb: an MCP **proxy** should forward schemas verbatim. Any place
  that rebuilds a tool schema from scratch is a flattening risk.
