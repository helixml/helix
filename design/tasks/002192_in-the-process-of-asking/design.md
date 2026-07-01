# Design: Replace Bot-Editing MCP Tools with Discrete Attach/Detach and Subscribe Operations

## Where the code lives

All in `api/pkg/org/`:

- MCP tools: `interfaces/mcptools/` — `create_bot.go`, `update_bot.go`
  (delete), `invite_bots.go` (delete), `subscribe.go`, `unsubscribe.go`,
  `schema.go`, `builtins.go` (registration), `registry.go`, `defaults.go`
  (`OwnerBotTools`/`BaseReadTools`).
- Application: `application/bots/bots.go` (`Create`, `Update`, `MergeTools`),
  `application/subscriptions/subscriptions.go`
  (`Subscribe`/`Unsubscribe` already take an explicit `workerID`).
- Domain: `domain/orgchart/bot.go` (`Bot`, `NewBot`, `WithTopics` — `Topics`
  retained).
- Persistence: `infrastructure/persistence/gorm/bot.go`, memory store.
- Read DTO / REST: `interfaces/mcptools/read_bots.go`,
  `interfaces/server/api/dto.go`, `interfaces/server/api/bots.go`.
- Types: `domain/tool/tool.go` (`type Name = string`),
  `domain/streaming/ids.go` (`type TopicID = string`) — aliases for `string`.

## The new MCP surface (all scalar args — no arrays)

| Tool | Args | Backed by |
|---|---|---|
| `create_bot` | `id?`, `content`, `topics` (non-nullable array), `parentId?` | `bots.Create` (baseline tools + topics manifest) |
| `set_bot_content` *(assumption)* | `botId`, `content` | `bots.Update{Content}` |
| `attach_tool` | `botId`, `tool` (enum) | new `bots.AttachTool` |
| `detach_tool` | `botId`, `tool` (enum) | new `bots.DetachTool` |
| `subscribe` | `botId`, `topicId` | `subscriptions.Subscribe(orgID, botId, topicId)` |
| `unsubscribe` | `botId`, `topicId` | `subscriptions.Unsubscribe(orgID, botId, topicId)` |

Removed: `update_bot`, `invite_bots`, and the caller-only forms of
`subscribe`/`unsubscribe`.

## Change A — `attach_tool` / `detach_tool` (discrete tool grants)

New MCP tools with scalar args:

```go
type attachToolArgs struct {
    BotID string `json:"botId"`
    Tool  string `json:"tool"`
}
```

Service methods on `bots.Bots` (keep the read-modify-write in the service, its
single home):
- `AttachTool(ctx, orgID, id, name)` — Get the Bot; if `name` already in
  `Tools`, no-op; else append and persist via the existing `Update` path.
  Validate `name` is a registered tool (see enum source below).
- `DetachTool(ctx, orgID, id, name)` — Get the Bot; if `name` is in
  `BaseReadTools`, refuse (mandatory baseline); if absent, no-op; else remove
  and persist.

### Enum of valid tool names (from the registry)
The `tool` argument is advertised as a required, non-nullable string enum of
registered tool names. Those names live in the `Registry` (populated by
`RegisterBuiltins`), not at package-init time — so the schema **cannot** be a
static `mustSchema` var.

- Add a names provider to `Deps` (e.g. `ToolNames func() []tool.Name`) wired in
  `RegisterBuiltins` to return `reg.List()` names.
- Build the enum in `InputSchema()` at serve time (called per ListTools
  request), so it always reflects the current registry and no registration-order
  problem exists (all tools are registered by the time any schema is served).
- Add a `schema.go` helper (e.g. `enumStringProperty(names, description)`)
  reusing the existing `enumSchema` shape.

This directly satisfies the earlier "make tools a discoverable enum" requirement
and keeps the core generic (new tools appear automatically).

## Change B — `subscribe` / `unsubscribe` target a Bot

Change both args from `{topicId}` (caller-only) to `{botId, topicId}` and pass
`orgchart.BotID(args.BotID)` as the worker id. `subscriptions.Subscribe` and
`Unsubscribe` already accept an explicit `workerID` and validate the Topic and
Bot exist, so this is a straight signature change in the MCP adapter — no
service change. A Bot self-subscribes by passing its own id.

Delete `invite_bots.go` (the bulk many-bots form) — one `subscribe` call per bot
replaces it. Update `OwnerBotTools` and any tests that referenced
`InviteBotsName`.

## Change C — `create_bot` keeps `topics` (fixed schema); remove `update_bot`

- `create_bot.go` — drop only `Tools` from `createBotArgs`; **keep `Topics`**.
  Args become `id`/`content`/`topics`/`parentId`. Update the description to
  point at the follow-ups (`attach_tool` for tools, `subscribe` for actual
  stream subscription, `set_bot_content` for content) and to explain that
  `topics` is a declarative manifest that does not itself subscribe.
  `bots.Create` builds the Bot with `MergeTools(nil, baseTools)` (baseline) plus
  the supplied topics manifest.
- **Fix the `topics` schema.** The auto-generated schema is the nullable union
  `{"type":["null","array"],"items":{"type":"string"}}`. Override the `topics`
  property to a non-nullable array `{"type":"array","items":{"type":"string"}}`
  and add `topics` to the schema's `required` list (so the model must pass `[]`
  for none). Add a `schema.go` helper (e.g. `stringArrayProperty(description)`)
  for the non-nullable array shape; set it on the property and append `topics`
  to `Required`. Drop `omitempty` on the Go field to match "always present".
  The `Invoke` may treat a nil slice as empty (forgiving) while the schema
  advertises it required.
- Delete `update_bot.go` and `UpdateBotName`; remove from `OwnerBotTools`,
  `builtins.go`, and tests.
- `set_bot_content.go` *(assumption)* — new tool, args `{botId, content}`,
  calls `bots.Update` with only `Content` set (preserves tools and topics).
- **Keep `Bot.Topics`** end-to-end (field, `WithTopics`, `NewBot` topics param,
  `bots.CreateParams.Topics`, `lifecycle.CreateParams.Topics`, GORM
  column/mapping, read DTO, REST DTOs). Note it stays a stored no-op manifest by
  design decision (not a subscription).

`bots.UpdateParams.Tools`/`.Topics` stay (Tools used by
`AttachTool`/`DetachTool` via the `Update` path). `create_bot` no longer takes
tools, so `bots.CreateParams.Tools` can be dropped (create applies baseline
only); `CreateParams.Topics` stays.

## Registration & authorization
- `builtins.go` — register `attach_tool`, `detach_tool`, `set_bot_content`;
  unregister `update_bot`, `invite_bots`.
- `defaults.go` `OwnerBotTools()` — replace `UpdateBotName`/`InviteBotsName`
  with `AttachToolName`, `DetachToolName`, `SetBotContentName`; keep
  `SubscribeName`/`UnsubscribeName`. These stay out of `BaseReadTools`
  (they are mutations). Authorization is by tool possession, as today.

## Key decisions
- **Discrete over bulk.** Scalar-only args eliminate the array-schema interop
  bug at the source and make each op trivially describable.
- **Enum from the registry**, built dynamically in `InputSchema()`, not a
  hand-maintained list — honors "new tools addable without editing the core."
- **`subscribe`/`unsubscribe` target a Bot** — the service already supports an
  explicit worker id, so `invite_bots` is redundant and removed.
- **Keep a content editor** (`set_bot_content`) so removing `update_bot` doesn't
  silently make content immutable. Drop it only if content is meant to be
  immutable.
- **Keep `topics` on `create_bot`** as a declarative manifest (per request),
  with its schema fixed to a non-nullable, required array. It remains a stored
  no-op (does not subscribe) by design; subscription is `subscribe`.

## Testing
- `attach_tool`/`detach_tool`: add a tool; idempotent re-add; detach a
  non-baseline tool; detach refuses a baseline tool; unknown tool rejected;
  resulting tool set order-stable (`MergeTools`).
- `subscribe`/`unsubscribe`: subscribe another Bot; self-subscribe; unsubscribe;
  unknown bot/topic rejected.
- `create_bot`: create with `topics: []` yields tools == `BaseReadTools` and an
  empty manifest; create with `topics: ["t1"]` stores that manifest; no `tools`
  field in the schema.
- `set_bot_content`: changes content, preserves tools and topics.
- Schema tests: `attach_tool.tool` advertises a non-nullable `enum`;
  `create_bot.topics` advertises a non-nullable, required array (no
  `["null","array"]` union); registry additions surface in the enum.
- `builtins_test.go` / `spec_tasks_registration_test.go`: reflect the
  registered/unregistered set; fix all `NewBot` call sites after the signature
  change; `go build ./...` for the org packages.
- Manual MCP smoke (inner Helix): `create_bot` → `attach_tool` `subscribe`/`dm`
  → `create_topic` → `subscribe(botId, topicId)` — the original failing flow.
</content>
