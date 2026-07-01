# fix(server): preserve upstream MCP schema through the external proxy

## Summary

`create_bot` and `subscribe` were uncallable by `zed_external` agents (org
Bots): their array params (`tools`, `topics`, `topicIds`) were advertised as
`type:"string"`, so passing a JSON array failed with
`cannot unmarshal string into []string`.

Root cause is the external MCP proxy, not the org server. `ExternalMCPBackend`
(the MCP proxy Zed connects to at `/api/v1/mcp/external/{name}`) rebuilt each
proxied tool's schema property-by-property as `mcp.WithString`, discarding
`type`, `items`, and `enum`. The org MCP server itself already publishes correct
arrays (verified by `TestSchemaWireArrayParams`). A prior fix (#2774) hardened a
different consumer (`pkg/agent/skill/mcp`, the in-process Go agent path) that
`zed_external` Bots never use — which is why rebuilding didn't help.

The fix forwards the upstream input schema verbatim, so arrays and nested
schemas survive. A proxy should transport schemas, not reinterpret them.

## Changes

- `api/pkg/server/mcp_backend_external.go`: extract `buildProxyTool` and forward
  the upstream schema verbatim via
  `mcp.NewToolWithRawSchema(name, description, json.Marshal(tool.InputSchema))`;
  on marshal error, log and serve a description-only tool (never an all-string
  schema). Adds the `encoding/json` import.
- `api/pkg/server/mcp_backend_external_schema_test.go`: regression test that
  calls the real `buildProxyTool` and asserts an upstream array param is
  re-advertised as `type:"array"` (fails against the old code, passes now).

## Testing

- `TestBuildProxyTool_PreservesArrayParams` — GREEN (RED against old code).
- `go test ./pkg/server/ -run 'MCP|External|Backend'` — ok.
- `go build ./pkg/server/` — clean. Org server `TestSchemaWireArrayParams` — ok.
- Note: `api/pkg/server` tests need `CGO_ENABLED=1` (tree-sitter).

Spec-Ref: 002204_the-one-blocker
