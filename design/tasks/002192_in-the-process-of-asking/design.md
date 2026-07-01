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
- Domain: `domain/orgchart/bot.go` (`Bot`, `NewBot`, `WithTopics` — drop
  `Topics`).
- Persistence: `infrastructure/persistence/gorm/bot.go`, memory store.
- Read DTO / REST: `interfaces/mcptools/read_bots.go`,
  `interfaces/server/api/dto.go`, `interfaces/server/api/bots.go`.
- Types: `domain/tool/tool.go` (`type Name = string`),
  `domain/streaming/ids.go` (`type TopicID = string`) — aliases for `string`.

## The new MCP surface (all scalar args — no arrays)

| Tool | Args | Backed by |
|---|---|---|
| `create_bot` | `id?`, `content`, `parentId?` | `bots.Create` (baseline tools only) |
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

## Change C — `create_bot` is bare; remove `update_bot`; remove `Bot.Topics`

- `create_bot.go` — drop `Tools` and `Topics` from `createBotArgs`; args become
  `id`/`content`/`parentId`. Update the description to point at the follow-ups
  (`attach_tool` for tools, `subscribe` for streams, `set_bot_content` for
  content). `bots.Create` builds the Bot with `MergeTools(nil, baseTools)` so
  the baseline is always present.
- Delete `update_bot.go` and `UpdateBotName`; remove from `OwnerBotTools`,
  `builtins.go`, and tests.
- `set_bot_content.go` *(assumption)* — new tool, args `{botId, content}`,
  calls `bots.Update` with only `Content` set (preserves tools).
- Remove `Bot.Topics` end-to-end: the field + `WithTopics` in
  `domain/orgchart/bot.go`, the `topics` parameter of `NewBot`, `Topics` in
  `bots.CreateParams`/`UpdateParams` and `lifecycle.CreateParams`, the GORM
  column/mapping (+ memory store), and `Topics` in the read DTO and REST DTOs
  (drop `toTopicIDs` if unused). GORM AutoMigrate won't drop the DB column — an
  orphaned unread column is harmless; note it, no migration.

`bots.UpdateParams.Tools` stays (used by `AttachTool`/`DetachTool` via the
`Update` path). `create_bot` no longer takes tools, so `CreateParams.Tools` can
also be dropped (create always applies baseline only).

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
- **Remove `Bot.Topics`** — a stored no-op that never subscribed anything.

## Testing
- `attach_tool`/`detach_tool`: add a tool; idempotent re-add; detach a
  non-baseline tool; detach refuses a baseline tool; unknown tool rejected;
  resulting tool set order-stable (`MergeTools`).
- `subscribe`/`unsubscribe`: subscribe another Bot; self-subscribe; unsubscribe;
  unknown bot/topic rejected.
- `create_bot`: content-only create yields tools == `BaseReadTools`; no
  `tools`/`topics` in the schema.
- `set_bot_content`: changes content, preserves tools.
- Schema tests: `attach_tool.tool` advertises a non-nullable `enum`; no
  `["null","array"]` union anywhere on these tools; registry additions surface
  in the enum.
- `builtins_test.go` / `spec_tasks_registration_test.go`: reflect the
  registered/unregistered set; fix all `NewBot` call sites after the signature
  change; `go build ./...` for the org packages.
- Manual MCP smoke (inner Helix): `create_bot` → `attach_tool` `subscribe`/`dm`
  → `create_topic` → `subscribe(botId, topicId)` — the original failing flow.
</content>
