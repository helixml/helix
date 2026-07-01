# Implementation Tasks: Replace Bot-Editing MCP Tools with Discrete Attach/Detach and Subscribe Operations

## Principle change
- [ ] Amend the org "No workflow in code / streams stay prompt-driven" rule in `helix/CLAUDE.md` (and any mirror in `.cursor/rules/`) toward "complete a user action in as few steps as possible"; note `create_bot` subscribes at creation.

## New discrete tool grants (attach_tool / detach_tool)
- [ ] Add `AttachTool(ctx, orgID, id, name)` and `DetachTool(ctx, orgID, id, name)` to `application/bots/bots.go` (read-modify-write via `Update`; `DetachTool` refuses a `BaseReadTools` entry; both idempotent).
- [ ] Add a names provider to the mcptools `Deps` (e.g. `ToolNames func() []tool.Name`) wired in `RegisterBuiltins` to return `reg.List()` names.
- [ ] Add a `schema.go` helper (e.g. `enumStringProperty(names, description)`) building a required, non-nullable string `enum` property.
- [ ] Create `attach_tool.go` (`attach_tool`, args `{botId, tool}`) building its schema from the names provider in `InputSchema()`; validate the tool name; call `bots.AttachTool`.
- [ ] Create `detach_tool.go` (`detach_tool`, args `{botId, tool}`) mirroring attach; call `bots.DetachTool`.

## Bot-targeted subscribe / unsubscribe
- [ ] Change `subscribe.go` args to `{botId, topicId}` and pass `botId` to `subscriptions.Subscribe`; update description.
- [ ] Change `unsubscribe.go` args to `{botId, topicId}` and pass `botId` to `subscriptions.Unsubscribe`; update description.
- [ ] Delete `invite_bots.go` and `InviteBotsName`; remove all references.

## create_bot sets tools + subscribes at creation; remove update_bot; content editor
- [ ] Keep **both** `Tools` and `Topics` on `createBotArgs`; args become `id`/`content`/`tools`/`topics`/`parentId`; update `CreateBot.Description()` (tools grants initial tools unioned with baseline, attach/detach to change later; topics subscribes the new bot at creation; `set_bot_content` for content).
- [ ] Build `create_bot`'s schema dynamically in `InputSchema()` from the names provider (like attach_tool). Add `schema.go` helpers: `enumStringArrayProperty(names, description)` for `tools` (array of enum items) and `stringArrayProperty(description)` for `topics` (non-nullable string array); add `tools` and `topics` to `required`; drop `omitempty` on the Go fields.
- [ ] In `lifecycle.Create`, subscribe the new bot to each topic in `CreateParams.Topics` via the shared `subscriptions.Subscribe` use case (inject it into `lifecycle`). Validate all topics exist **before** writing the bot row (no partial creates).
- [ ] Delete `update_bot.go` and `UpdateBotName`; remove all references.
- [ ] Create `set_bot_content.go` (`set_bot_content`, args `{botId, content}`) calling `bots.Update` with only `Content` set.
- [ ] Keep `bots.CreateParams.Tools` and `.Topics`; `bots.Create` applies `MergeTools(p.Tools, baseTools)` and no longer stores a topics field on the bot.

## Remove Bot.Topics end-to-end (subscriptions are the source of truth)
- [ ] Remove the `Topics` field and `WithTopics` from `orgchart.Bot`, and the `topics` parameter from `NewBot(...)`; update every call site (production + tests).
- [ ] Drop `bots.UpdateParams.Topics` and its use in `Update`.
- [ ] Remove the `Topics` field and mapping from the GORM `botRow` (+ memory store). This stops all reads/writes of the redundant `org_bots.topics` column (`org_subscriptions` is the source of truth).
- [ ] Decide on the physical column: leave `org_bots.topics` orphaned (default — AutoMigrate won't drop it), or add a deliberate one-off `ALTER TABLE org_bots DROP COLUMN topics` for tidiness. Do **not** backfill old manifest entries into `org_subscriptions`.
- [ ] Remove `Topics` from the read DTO (`read_bots.go`) and the REST DTOs (`dto.go`/`bots.go`); drop `toTopicIDs`/`toToolNames` helpers if now unused.

## Registration & authorization
- [ ] In `builtins.go`, register `attach_tool`, `detach_tool`, `set_bot_content`; unregister `update_bot` and `invite_bots`.
- [ ] In `defaults.go` `OwnerBotTools()`, replace `UpdateBotName`/`InviteBotsName` with `AttachToolName`, `DetachToolName`, `SetBotContentName`; keep `SubscribeName`/`UnsubscribeName`.

## Tests & verification
- [ ] `attach_tool`/`detach_tool`: add; idempotent re-add; detach non-baseline; detach refuses baseline; unknown tool rejected; order-stable result.
- [ ] `subscribe`/`unsubscribe`: subscribe another bot; self-subscribe; unsubscribe; unknown bot/topic rejected.
- [ ] `create_bot`: `tools:["subscribe","dm"], topics:["t1","t2"]` (existing) → bot with tools ∪ baseline + a subscription row per topic; `tools:[], topics:[]` → tools == `BaseReadTools`, no subs; unknown topic → error and no bot row created; unknown tool name rejected; `tools` a required non-nullable enum array and `topics` a required non-nullable array.
- [ ] `set_bot_content`: content changes, tools preserved.
- [ ] Schema tests: `attach_tool.tool` non-nullable `enum`; `create_bot.tools` required non-nullable array of enum items; `create_bot.topics` required non-nullable array; no `["null","array"]` union remains; registry additions appear in both enums.
- [ ] Update `builtins_test.go` / `spec_tasks_registration_test.go`; fix `NewBot` call sites; `go build ./...` for the org packages.
- [ ] Manual MCP smoke (inner Helix): `create_topic` → `create_bot(topics:[…])`, publish to a listed topic and confirm the bot receives it → `attach_tool` (`subscribe`/`dm`) → `subscribe(botId, topicId)` for a later topic.
</content>
