# Requirements: Fix External MCP Proxy Flattening Array Params to String (create_bot/subscribe)

## Problem

A Bot in the org runtime re-loaded the `create_bot`/`subscribe` schemas fresh
and reports `tools`, `topics`, and `topicIds` are still `{"type":"string"}`,
not `type:"array"`. Passing a real JSON array serialises to a string, and the
Go handler rejects it: `cannot unmarshal string into []string`. So no Bot can
be created or subscribed. A prior fix (#2774) was rebuilt from scratch and did
**not** resolve it.

## Root cause (verified this session, empirically)

The previous fix (#2774) hardened the **wrong consumer**. There are two
separate MCP consumption paths, and org Bots use the one #2774 never touched:

1. The org MCP **server** (`/orgs/{org}/workers/{id}/mcp`) advertises the
   array params correctly as `type:"array", items:{type:"string"}`. **Proven**:
   `TestSchemaWireArrayParams` passes at HEAD (real go-sdk `tools/list`
   round-trip). The server is NOT the defect.

2. Org Bots run as **`zed_external`** agents (Zed IDE + Qwen Code). Their HTTP
   MCP servers are routed through the Helix proxy
   `ExternalMCPBackend` (`/api/v1/mcp/external/{name}`,
   `api/pkg/server/mcp_backend_external.go`). Zed connects to this proxy; the
   proxy is an MCP client to the org server and an MCP server to Zed.

   **The bug is in that proxy's re-advertisement.** When it rebuilds each
   tool's schema for Zed (`mcp_backend_external.go`, ~L322–358), it emits every
   property with `mcp.WithString(...)` regardless of the upstream type. It
   discards `type`, `items`, `enum`, and nested `properties`. So
   `create_bot.tools`, `create_bot.topics`, and `subscribe.topicIds` — all
   arrays upstream — are re-published to Zed as `type:"string"`, reproducing the
   Bot's exact symptom.

The call-forwarding handler (`createToolHandler`) forwards arguments verbatim,
so once the advertised schema is correct, real arrays flow through unchanged
and the org handler accepts them. The schema advertisement is the only defect.

#2774 fixed `buildParameters`/`convertMapToDefinition` in
`api/pkg/agent/skill/mcp/mcp_skill.go` — the Go agent's *OpenAI* tool path.
That path is not used by `zed_external` Bots, which is why the rebuild changed
nothing.

## User Stories

- **As the reporting Bot**, when I load the `create_bot`/`subscribe` schemas I
  want `tools`/`topics`/`topicIds` to read `type:"array", items:string`, so I
  can pass real arrays and `create_bot`/`subscribe` succeed in one call.
- **As any user of a `zed_external` agent** with an external HTTP MCP server, I
  want array/object/number/boolean tool parameters preserved through the proxy,
  not silently flattened to string.

## Acceptance Criteria

1. `ExternalMCPBackend` re-advertises each proxied tool with its **full**
   upstream input schema (type, items, enum, required, nested properties)
   preserved — not a per-property `WithString` rebuild.
2. Through the external proxy, `create_bot.tools`, `create_bot.topics`,
   `subscribe.topicIds` (and `attach_tool.tools`, `detach_tool.tools`,
   `unsubscribe.topicIds`) advertise `type:"array"` with `items.type:"string"`.
3. Non-array params (e.g. `create_bot.content`, `parentId`, `subscribe.botId`)
   remain correct, and `required` is preserved.
4. A tool call through the proxy passing real JSON arrays reaches the org
   handler as arrays and succeeds (no `cannot unmarshal string into []string`).
5. A regression test asserts the proxy preserves a non-string param type
   (fails against the old `WithString`-only code).
6. Existing external-MCP tests still pass; `go build ./...` clean.

## Out of Scope

- The org MCP server schema and #2774's `mcp_skill.go` change (already correct).
- Changing the `create_bot`/`subscribe` handler contract or arguments.
- Redeploying the reporting Bot's runtime (a separate rollout step once merged).
