# Design: Replace Bot-Editing MCP Tools with Bulk Attach/Detach and Subscribe Operations

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
| `create_bot` | `id?`, `content`, `tools` (enum array), `topics` (array), `parentId?` | `lifecycle.Create` → bot (tools ∪ baseline) + a subscription row per topic |
| `set_bot_content` | `botId`, `content` | `bots.Update{Content}` |
| `delete_bot` | `botId` | `lifecycle.Delete` (cascades) |
| `attach_tool` | `botId`, `tools` (enum array) | new `bots.AttachTools` |
| `detach_tool` | `botId`, `tools` (enum array) | new `bots.DetachTools` |
| `subscribe` | `botId`, `topicIds` (array) | new `subscriptions.SubscribeTopics(orgID, botId, topicIds)` |
| `unsubscribe` | `botId`, `topicIds` (array) | new `subscriptions.UnsubscribeTopics(orgID, botId, topicIds)` |

Removed: `update_bot`, `invite_bots`, the caller-only `subscribe`/`unsubscribe`.

Unchanged (existing baseline read tools, reused as-is): `list_bots`, `get_bot`
(`read_bots.go`), `list_topics`, `get_topic` (`read_streams.go`). No
`list_tools`/`get_tool` — the tool catalogue is already in the system prompt and
the MCP tool-list API.

## Change A — `attach_tool` / `detach_tool` (tool grants, array + enum)

```go
type attachToolArgs struct {
    BotID string   `json:"botId"`
    Tools []string `json:"tools"`
}
```

Both take a **non-nullable array** of tool names (pass one or many), each item
constrained to the registered-tool-name enum. Service methods on `bots.Bots`
(keep the read-modify-write in the service):
- `AttachTools(ctx, orgID, id, names)` — Get the Bot; union `names` into `Tools`
  (idempotent per name, order-stable via `MergeTools`); persist via `Update`.
  Validate every name is registered up front.
- `DetachTools(ctx, orgID, id, names)` — Get the Bot; refuse if any name is in
  `BaseReadTools`; remove those present (idempotent per name); persist. Validate
  the whole batch first so a bad/baseline name fails the call before any write.

### Enum of valid tool names (from the registry)
The `tools` items are the registered tool names as a JSON-Schema `enum`. Those
names live in the `Registry` (populated by `RegisterBuiltins`), not at
package-init — so these schemas can't be static `mustSchema` vars.
- Add a names provider to `Deps` (e.g. `ToolNames func() []tool.Name`) wired in
  `RegisterBuiltins` to return `reg.List()` names.
- Build the schema in `InputSchema()` at serve time (per ListTools request) so
  it always reflects the current registry.
- Reuse the same `schema.go` helper `create_bot` uses for its `tools`
  (e.g. `enumStringArrayProperty(names, description)`): a non-nullable array of
  enum-constrained strings; add `tools` to `required`.

## Change B — `subscribe` / `unsubscribe` target a Bot, take a topic array

Change both args from `{topicId}` (caller-only) to `{botId, topicIds}`, where
`topicIds` is a **non-nullable array** (add/remove one or many at once). Add
batch service methods that validate the Bot + every Topic up front (like
`subscriptions.Invite` does) then apply idempotently per topic, reusing the
existing single `Subscribe`/`Unsubscribe` primitives (DRY):
- `SubscribeTopics(ctx, orgID, botID, topicIDs)`
- `UnsubscribeTopics(ctx, orgID, botID, topicIDs)`

Topic ids are dynamic values (not a fixed set), so `topicIds` items are plain
strings — **no enum**: a non-nullable string array via `stringArrayProperty`,
added to `required`. Delete `invite_bots.go` and `InviteBotsName`;
`subscribe(botId, topicIds)` covers the create/subscribe flows (subscribe many
bots by calling it per bot).

## Change C — `create_bot` subscribes immediately; remove `update_bot` and `Bot.Topics`

- `create_bot.go` — keep **both** `Tools` and `Topics` on `createBotArgs`.
  Args become `id`/`content`/`tools`/`topics`/`parentId`. Both are required,
  non-nullable arrays (drop `omitempty`; add both to the schema `required`);
  override the auto-generated `["null","array"]` unions:
  - `tools`: an array whose items are the registered-tool-name enum
    (`{"type":"array","items":{"type":"string","enum":[…]}}`), built from the
    names provider — so `create_bot`'s schema is **dynamic** (built in
    `InputSchema()`), like `attach_tool`/`detach_tool`. Add a `schema.go` helper
    (e.g. `enumStringArrayProperty(names, description)`).
  - `topics`: a non-nullable string array
    (`{"type":"array","items":{"type":"string"}}`) via a `stringArrayProperty`
    helper.
  Update the description: `tools` grants the Bot's initial tools (unioned with
  the read baseline; `attach_tool`/`detach_tool` change them later); `topics`
  subscribes the new Bot to each listed (existing) topic at creation.
- **`lifecycle.Create` performs the subscription, reusing the same use case.**
  After creating the bot row, call the **same**
  `subscriptions.SubscribeTopics(orgID, botID, CreateParams.Topics)` batch the
  `subscribe` tool calls — one subscription implementation, no duplicated logic
  (DRY). Inject the subscriptions service into `lifecycle` so the REST create
  path shares the behavior. Note the separation of concerns: internally create
  reuses subscribe; externally `create_bot` and `subscribe` stay two distinct
  operations.
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
  field on the GORM `botRow` mapper (+ memory store), and `Topics` from the read
  DTO (`read_bots.go`) and REST DTOs (`dto.go`/`bots.go`). The
  `org_bots.topics` column is a redundant JSON array duplicating the
  `org_subscriptions` association table (PK `org_id, bot_id, topic_id`);
  `org_subscriptions` is the single source of truth (queried via the
  subscriptions/topic read tools). Removing the field stops all reads/writes of
  the column — that is the cleanup that matters.

### Data migration
- **The physical column stays unless explicitly dropped.** GORM AutoMigrate
  only adds columns, so `org_bots.topics` remains as orphaned, unread data.
  Per the "GORM AutoMigrate only" convention, the default is to leave it. Drop
  it only if desired via a deliberate one-off `ALTER TABLE org_bots DROP COLUMN
  topics` (outside AutoMigrate) — purely tidiness, no functional effect.
- **Do NOT backfill the old manifest into `org_subscriptions`.** The `topics`
  column was a no-op manifest — its entries were never real subscriptions, so a
  bot may list a topic there with no `org_subscriptions` row. Backfilling would
  silently subscribe existing bots to streams they never received (a behavior
  change). Existing bots keep their current (real) subscriptions unchanged;
  anything that needs a subscription gets one explicitly via `subscribe`.
- `bots.CreateParams.Tools`/`.Topics` and `lifecycle.CreateParams.Topics`
  **stay**: Tools is unioned with the baseline (`MergeTools(p.Tools,
  baseTools)`), Topics drives subscription. `bots.Create` no longer stores a
  topics field on the bot. Drop `bots.UpdateParams.Topics` (nothing writes a bot
  topics field anymore); keep `bots.UpdateParams.Tools` (used by attach/detach).

## Change D — `delete_bot`; reads already exist

- **`delete_bot.go`** — new tool, args `{botId}`, wraps `lifecycle.Delete(orgID,
  botID)` (the same use case REST `DELETE /bots/{id}` calls). `lifecycle.Delete`
  already stops sessions, deletes the Helix project + agent app, clears runtime
  state, cascades subscriptions + reporting lines, deletes the bot row, and
  reconciles team/DM topics; activations are preserved as audit. No service
  change. Owner mutation.
- **List/get already exist** — `list_bots`/`get_bot` and `list_topics`/
  `get_topic` are baseline read tools; reuse as-is. (`read_bots.go` drops the
  `topics` field per Change C; list/get are otherwise unchanged.)
- **No `list_tools`/`get_tool`** — the tool catalogue is already provided via the
  system prompt and the MCP tool-list API.

## Registration & authorization
- `builtins.go` — register `attach_tool`, `detach_tool`, `set_bot_content`,
  `delete_bot`; unregister `update_bot`, `invite_bots`.
- `defaults.go` `OwnerBotTools()` — replace `UpdateBotName`/`InviteBotsName`
  with `AttachToolName`, `DetachToolName`, `SetBotContentName`, `DeleteBotName`;
  keep `SubscribeName`/`UnsubscribeName`. These stay out of `BaseReadTools`
  (`list_bots`/`get_bot`/`list_topics`/`get_topic` remain the baseline reads).
  Authorization is by tool possession, as today.

## Key decisions
- **Bulk, not per-item.** Grant/revoke tools and subscribe/unsubscribe topics
  accept arrays so a Bot can be set up in one call — per-item granting is too
  many hops. `attach_tool`/`detach_tool` and `create_bot` take non-nullable
  arrays of enum-constrained tool names (pass one or many); the enum makes
  values discoverable and the non-nullable override (not going scalar) is what
  avoids the `["null","array"]` union that caused the original bug.
  `subscribe`/`unsubscribe` likewise take a non-nullable `topicIds` array
  (plain strings, no enum — topic ids are dynamic).
- **`create_bot` sets initial tools and auto-subscribes** — per the amended
  "fewest steps" principle; the manager's common intent is a Bot that already
  has its tools and is listening. `tools` is an enum array (discoverable, same
  source as `attach_tool`); subscription reuses the one `subscriptions.Subscribe`
  use case (DRY). Externally `create_bot`, `attach_tool`/`detach_tool`, and
  `subscribe`/`unsubscribe` remain separate tools for later per-item edits.
- **Remove `Bot.Topics`**, don't rename it — subscriptions already live as rows;
  a parallel field would be a second source of truth that drifts.
- **Validate topics before writing the bot** — no partial creates.
- **Keep `set_bot_content`** so content stays editable after `update_bot` goes.

## Testing
- `attach_tool`/`detach_tool`: attach a multi-tool array; idempotent re-add;
  detach a subset; detach refuses if any name is baseline; an unknown tool in
  the array is rejected (whole call fails, no partial write); order-stable
  (`MergeTools`).
- `subscribe`/`unsubscribe`: subscribe a bot to a topic array; self-subscribe;
  unsubscribe a subset; unknown bot or any unknown topic in the array rejected
  (whole call fails, no partial write); idempotent per topic.
- `create_bot`: `tools:["subscribe","dm"], topics:["t1","t2"]` (existing) → bot
  with tools ∪ baseline + a subscription row per topic; `tools:[], topics:[]` →
  tools == `BaseReadTools`, no subscriptions; an unknown topic → error and
  **no** bot row created (atomicity); an unknown tool name → rejected; `tools`
  advertised as a required non-nullable enum array, `topics` as a required
  non-nullable array.
- `set_bot_content`: content changes, tools preserved.
- `delete_bot`: deletes an existing bot (its subscriptions + reporting lines
  gone, reports become parentless); deleting an absent bot → not-found error.
  `list_bots`/`get_bot`/`list_topics`/`get_topic` still work (regression check).
- Schema tests: `attach_tool.tools`/`detach_tool.tools` and `create_bot.tools`
  are required non-nullable arrays of enum items; `subscribe.topicIds`/
  `unsubscribe.topicIds` and `create_bot.topics` are required non-nullable
  string arrays; no `["null","array"]` union remains; registry additions appear
  in the enums.
- `builtins_test.go` / `spec_tasks_registration_test.go`: registered set;
  fix `NewBot` call sites; `go build ./...` for the org packages.
- Manual MCP smoke (inner Helix): `create_topic` → `create_bot(topics:[…])` and
  confirm the Bot receives an event published to a listed topic; `attach_tool`
  `[subscribe, dm]`; `subscribe(botId, [topicId, …])` for later topics.
</content>

## Implementation Notes (post-build)

- **Org tests use the in-memory store**, not Postgres: `gorm.GetOrgTestDB` returns
  `memory.New()`. So `CGO_ENABLED=0 go test ./pkg/org/...` runs the whole org
  suite with no DB. CGO is only needed for `pkg/server` tests (tree-sitter); the
  build itself is CGO-free.
- **`tool.Name` and `streaming.TopicID` are `= string` aliases**, so `[]string`
  is assignable to `[]tool.Name`/`[]streaming.TopicID` — the tool args use
  `[]string` and pass straight through to the services.
- **Dynamic enum schema:** `Deps.ToolNames` is a closure wired in
  `RegisterBuiltins` that reads `reg.List()` lazily, so `InputSchema()` sees every
  registered tool regardless of registration order. `withProperty` clones the
  reflected base schema's Properties map so concurrent `InputSchema()` calls never
  mutate shared state.
- **`create_bot` subscription lives in `lifecycle.Create`** (not the MCP adapter)
  so REST `POST /bots` gets it too; it reuses `subscriptions.SubscribeTopics`
  (DRY). Topics are validated before the bot row is written → no partial creates.
- **Merge with `main`:** main added a `PreserveContext` bool to the Bot end-to-end
  concurrently. Resolved by keeping both (PreserveContext threaded through;
  `Topics` removed). The `org_bots.topics` column is left orphaned; `NewBot` lost
  its `topics` param (≈45 call sites updated, mostly tests).
- **Frontend needed no changes:** the bot detail page reads subscriptions from
  `useListBotSubscriptions` (subscription rows), never `bot.topics`. Regenerated
  swagger + TS client only dropped the two `topics` fields.
- **Kept `subscriptions.Invite`** (many-bots→one-topic): still has unit tests and
  is the orthogonal counterpart to `SubscribeTopics`; not removed despite
  `invite_bots` being gone.
