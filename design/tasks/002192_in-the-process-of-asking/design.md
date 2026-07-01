# Design: Replace Bot-Editing MCP Tools with Discrete Attach/Detach and Subscribe Operations

## Where the code lives

All in `api/pkg/org/`:

- MCP tools: `interfaces/mcptools/` — `create_bot.go`, `update_bot.go`
  (delete), `invite_bots.go` (delete), `subscribe.go`, `unsubscribe.go`,
  `schema.go`, `builtins.go` (registration), `registry.go`, `defaults.go`
  (`OwnerBotTools`/`BaseReadTools`).
- Application: `application/bots/bots.go` (`Create`, `Update`, `MergeTools`),
  `application/subscriptions/subscriptions.go`
  (`Subscribe`/`Unsubscribe`/`Invite` — already take an explicit `workerID`),
  `application/lifecycle/lifecycle.go` (`Create` orchestrates bot creation).
- Domain: `domain/orgchart/bot.go` (`Bot`, `NewBot`, `WithTopics` — remove
  `Topics`).
- Persistence: `infrastructure/persistence/gorm/bot.go`, memory store.
- Read DTO / REST: `interfaces/mcptools/read_bots.go`,
  `interfaces/server/api/dto.go`, `interfaces/server/api/bots.go`.
- Types: `domain/tool/tool.go` (`type Name = string`),
  `domain/streaming/ids.go` (`type TopicID = string`) — aliases for `string`.
- Principle text: `helix/CLAUDE.md` (org "No workflow in code" section) — amend.

## Principle change

The org-package rule "**No workflow in code** … does not subscribe Workers …
`Role.Streams` stays prompt-driven" is amended toward **complete a user action
in as few steps as possible**. `create_bot` may therefore create subscriptions
as part of creation. Update the `helix/CLAUDE.md` text (and any mirror in
`.cursor/rules/`) so the codebase principle and the code agree. `subscribe`
stays as the tool for subscribing *after* creation.

## The new MCP surface

| Tool | Args | Backed by |
|---|---|---|
| `create_bot` | `id?`, `content`, `topics` (non-nullable array), `parentId?` | `lifecycle.Create` → bot (baseline tools) + a subscription row per topic |
| `set_bot_content` | `botId`, `content` | `bots.Update{Content}` |
| `attach_tool` | `botId`, `tool` (enum) | new `bots.AttachTool` |
| `detach_tool` | `botId`, `tool` (enum) | new `bots.DetachTool` |
| `subscribe` | `botId`, `topicId` | `subscriptions.Subscribe(orgID, botId, topicId)` |
| `unsubscribe` | `botId`, `topicId` | `subscriptions.Unsubscribe(orgID, botId, topicId)` |

Removed: `update_bot`, `invite_bots`, the caller-only `subscribe`/`unsubscribe`.

## Change A — `attach_tool` / `detach_tool` (discrete tool grants)

```go
type attachToolArgs struct {
    BotID string `json:"botId"`
    Tool  string `json:"tool"`
}
```

Service methods on `bots.Bots` (keep the read-modify-write in the service):
- `AttachTool(ctx, orgID, id, name)` — Get the Bot; append `name` if absent
  (idempotent); persist via `Update`. Validate `name` is registered.
- `DetachTool(ctx, orgID, id, name)` — Get the Bot; refuse if `name` is in
  `BaseReadTools`; remove if present (idempotent); persist.

### Enum of valid tool names (from the registry)
The `tool` arg is a required, non-nullable string enum of registered tool names.
Those names live in the `Registry` (populated by `RegisterBuiltins`), not at
package-init — so the schema can't be a static `mustSchema` var.
- Add a names provider to `Deps` (e.g. `ToolNames func() []tool.Name`) wired in
  `RegisterBuiltins` to return `reg.List()` names.
- Build the enum in `InputSchema()` at serve time (per ListTools request) so it
  always reflects the current registry.
- Add a `schema.go` helper (e.g. `enumStringProperty(names, description)`)
  reusing the existing `enumSchema` shape.

## Change B — `subscribe` / `unsubscribe` target a Bot

Change both args from `{topicId}` (caller-only) to `{botId, topicId}` and pass
`orgchart.BotID(args.BotID)` as the worker id. `subscriptions.Subscribe` /
`Unsubscribe` already accept an explicit `workerID` and validate the Topic and
Bot exist — a straight signature change in the MCP adapter, no service change.
Delete `invite_bots.go` and `InviteBotsName`; one `subscribe` per bot replaces
it.

## Change C — `create_bot` subscribes immediately; remove `update_bot` and `Bot.Topics`

- `create_bot.go` — drop `Tools` from `createBotArgs`; keep `Topics` as the
  input list. Args become `id`/`content`/`topics`/`parentId`. Fix the `topics`
  schema: override the auto-generated union
  (`{"type":["null","array"],…}`) to a non-nullable array
  `{"type":"array","items":{"type":"string"}}` and add `topics` to `required`;
  drop `omitempty` on the Go field. Add a `schema.go` helper
  (e.g. `stringArrayProperty(description)`). Update the description: `topics`
  subscribes the new Bot to each listed (existing) topic at creation.
- **`lifecycle.Create` performs the subscription, reusing the same use case.**
  After creating the bot row, loop `CreateParams.Topics` and call the **same**
  `subscriptions.Subscribe(orgID, botID, topicID)` method the `subscribe` tool
  calls — one subscription implementation, no duplicated logic (DRY). Inject the
  subscriptions service into `lifecycle` so the REST create path shares the
  behavior. Note the separation of concerns: internally create reuses subscribe;
  externally `create_bot` and `subscribe` stay two distinct operations.
  - **Atomicity:** validate every topic exists *before* writing the bot row
    (as `subscriptions.Invite` already validates up front), so a bad topic id
    fails the call with no partially-created Bot. Then create the bot and its
    subscription rows.
- Delete `update_bot.go` and `UpdateBotName`; remove from `OwnerBotTools`,
  `builtins.go`, tests.
- `set_bot_content.go` — new tool, args `{botId, content}`, `bots.Update` with
  only `Content` set (preserves tools).
- **Remove `Bot.Topics` end-to-end** — the field + `WithTopics` in
  `domain/orgchart/bot.go`, the `topics` parameter of `NewBot`, the `Topics`
  field on the GORM model (+ memory store), and `Topics` from the read DTO
  (`read_bots.go`) and REST DTOs (`dto.go`/`bots.go`). Subscriptions are the
  source of truth (queried via the subscriptions/topic read tools). GORM
  AutoMigrate won't drop the DB column — an orphaned unread column is harmless;
  note it, no migration.
- `bots.CreateParams.Topics` and `lifecycle.CreateParams.Topics` **stay** (they
  now drive subscription, not a stored field). `bots.Create` no longer stores
  topics on the bot and applies `MergeTools(nil, baseTools)`. Drop
  `bots.CreateParams.Tools` (create applies baseline only) and
  `bots.UpdateParams.Topics` (nothing writes a bot topics field anymore);
  keep `bots.UpdateParams.Tools` (used by attach/detach).

## Registration & authorization
- `builtins.go` — register `attach_tool`, `detach_tool`, `set_bot_content`;
  unregister `update_bot`, `invite_bots`.
- `defaults.go` `OwnerBotTools()` — replace `UpdateBotName`/`InviteBotsName`
  with `AttachToolName`, `DetachToolName`, `SetBotContentName`; keep
  `SubscribeName`/`UnsubscribeName`. These stay out of `BaseReadTools`.
  Authorization is by tool possession, as today.

## Key decisions
- **Discrete over bulk** for tool-granting/subscription — scalar args kill the
  array-schema bug at the source; `tool` is a discoverable enum.
- **`create_bot` auto-subscribes** — per the amended "fewest steps" principle;
  the manager's most common intent is a Bot that's listening immediately.
  Internally it reuses the one `subscriptions.Subscribe` use case (DRY);
  externally `create_bot` and `subscribe` remain two separate tools.
- **Remove `Bot.Topics`**, don't rename it — subscriptions already live as rows;
  a parallel field would be a second source of truth that drifts.
- **Validate topics before writing the bot** — no partial creates.
- **Keep `set_bot_content`** so content stays editable after `update_bot` goes.

## Testing
- `attach_tool`/`detach_tool`: add; idempotent; detach non-baseline; detach
  refuses baseline; unknown tool rejected; order-stable (`MergeTools`).
- `subscribe`/`unsubscribe`: subscribe another bot; self-subscribe; unsubscribe;
  unknown bot/topic rejected.
- `create_bot`: `topics: ["t1","t2"]` (existing) → bot created + a subscription
  row per topic; `topics: []` → bot, no subscriptions, tools == `BaseReadTools`;
  an unknown topic → error and **no** bot row created (atomicity); no `tools`
  field in the schema; `topics` advertised as a required non-nullable array.
- `set_bot_content`: content changes, tools preserved.
- Schema tests: `attach_tool.tool` is a non-nullable `enum`; `create_bot.topics`
  is a required non-nullable array; no `["null","array"]` union remains.
- `builtins_test.go` / `spec_tasks_registration_test.go`: registered set;
  fix `NewBot` call sites; `go build ./...` for the org packages.
- Manual MCP smoke (inner Helix): `create_topic` → `create_bot(topics:[…])` and
  confirm the Bot receives an event published to a listed topic; `attach_tool`
  `subscribe`/`dm`; `subscribe(botId, topicId)` for a later topic.
</content>
