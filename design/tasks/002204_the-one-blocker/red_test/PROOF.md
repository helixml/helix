# RED/GREEN proof — external MCP proxy array flattening

This time the test exercises the **real production code path**, not a replica.
`buildProxyTool` is extracted from `ExternalMCPBackend.getOrCreateServer`
(`api/pkg/server/mcp_backend_external.go`) — the exact function that builds the
schema advertised to a `zed_external` org Bot — and the test calls it directly.

Both runs use `CGO_ENABLED=1` (the package needs tree-sitter).

## RED — current production reconstruction (WithString loop)

Command: `CGO_ENABLED=1 go test ./pkg/server/ -run TestBuildProxyTool_PreservesArrayParams -v`

```
=== RUN   TestBuildProxyTool_PreservesArrayParams
    mcp_backend_external_schema_test.go:78: create_bot.tools re-advertised as "string", want "array" — the proxy flattened the array to string (cannot unmarshal string into []string)
--- FAIL: TestBuildProxyTool_PreservesArrayParams (0.00s)
FAIL
FAIL	github.com/helixml/helix/api/pkg/server	0.028s
```

This is the reported blocker, reproduced against real production code.

## GREEN — after the passthrough fix (NewToolWithRawSchema)

```
=== RUN   TestBuildProxyTool_PreservesArrayParams
--- PASS: TestBuildProxyTool_PreservesArrayParams (0.00s)
PASS
ok  	github.com/helixml/helix/api/pkg/server	0.029s
```

`go build ./pkg/server/` is clean. The scalar `content` param stays
`type:"string"`; only the array is corrected.

## Turnkey artifacts (apply at implementation)

- `mcp_backend_external_schema_test.go` — the regression test; drop into
  `api/pkg/server/`.
- `production_fix.patch` — the production change (extract `buildProxyTool`,
  wire `getOrCreateServer` to it, replace the WithString loop with a verbatim
  `NewToolWithRawSchema` passthrough, add the `encoding/json` import). Apply
  with `git apply` from the helix repo root.

Kept out of the helix repo during the planning phase; both were authored and
run there to produce the proof above, then reverted so helix stays clean until
the plan is approved.
