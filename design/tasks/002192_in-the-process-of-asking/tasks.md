# Implementation Tasks: Replace Bot-Editing MCP Tools with Bulk Attach/Detach and Subscribe Operations

## Principle change
- [x] Amend the org "No workflow in code / streams stay prompt-driven" rule in `helix/CLAUDE.md` (and any mirror in `.cursor/rules/`) toward "complete a user action in as few steps as possible"; note `create_bot` subscribes at creation.

## New bulk tool grants (attach_tool / detach_tool)
- [x] Add `AttachTools(ctx, orgID, id, names)` and `DetachTools(ctx, orgID, id, names)` to `application/bots/bots.go` (read-modify-write via `Update`; union/remove per name via `MergeTools`; `DetachTools` refuses any `BaseReadTools` entry; validate the whole batch up front; idempotent per name).
- [x] Add a names provider to the mcptools `Deps` (e.g. `ToolNames func() []tool.Name`) wired in `RegisterBuiltins` to return `reg.List()` names.
- [x] Add a `schema.go` helper (e.g. `enumStringArrayProperty(names, description)`) building a required, non-nullable array of enum-constrained strings (shared with `create_bot.tools`).
- [x] Create `attach_tool.go` (`attach_tool`, args `{botId, tools}`) building its schema from the names provider in `InputSchema()`; validate the tool names; call `bots.AttachTools`.
- [x] Create `detach_tool.go` (`detach_tool`, args `{botId, tools}`) mirroring attach; call `bots.DetachTools`.

## Bot-targeted subscribe / unsubscribe (topic arrays)
- [x] Add `SubscribeTopics(ctx, orgID, botID, topicIDs)` and `UnsubscribeTopics(ctx, orgID, botID, topicIDs)` to `application/subscriptions/subscriptions.go` — validate the Bot + every Topic up front (like `Invite`), then apply idempotently per topic via the existing single `Subscribe`/`Unsubscribe` primitives (DRY).
- [x] Change `subscribe.go` args to `{botId, topicIds}` (non-nullable array via `stringArrayProperty`, added to `required`); call `SubscribeTopics`; update description.
- [x] Change `unsubscribe.go` args to `{botId, topicIds}`; call `UnsubscribeTopics`; update description.
- [x] Delete `invite_bots.go` and `InviteBotsName`; remove all references.

## create_bot sets tools + subscribes at creation; remove update_bot; content editor
- [x] Keep **both** `Tools` and `Topics` on `createBotArgs`; args become `id`/`content`/`tools`/`topics`/`parentId`; update `CreateBot.Description()` (tools grants initial tools unioned with baseline, attach/detach to change later; topics subscribes the new bot at creation; `set_bot_content` for content).
- [x] Build `create_bot`'s schema dynamically in `InputSchema()` from the names provider (like attach_tool). Add `schema.go` helpers: `enumStringArrayProperty(names, description)` for `tools` (array of enum items) and `stringArrayProperty(description)` for `topics` (non-nullable string array); add `tools` and `topics` to `required`; drop `omitempty` on the Go fields.
- [x] In `lifecycle.Create`, subscribe the new bot to `CreateParams.Topics` via the shared `subscriptions.SubscribeTopics` batch (inject the subscriptions service into `lifecycle`). Validate all topics exist **before** writing the bot row (no partial creates).
- [x] Delete `update_bot.go` and `UpdateBotName`; remove all references.
- [x] Create `set_bot_content.go` (`set_bot_content`, args `{botId, content}`) calling `bots.Update` with only `Content` set.
- [x] Keep `bots.CreateParams.Tools` and `.Topics`; `bots.Create` applies `MergeTools(p.Tools, baseTools)` and no longer stores a topics field on the bot.

## delete_bot (list/get already exist)
- [x] Create `delete_bot.go` (`delete_bot`, args `{botId}`) wrapping `lifecycle.Delete(orgID, botID)` (same use case as REST `DELETE /bots/{id}`); no service change.
- [x] Reuse existing baseline reads `list_bots`/`get_bot` (`read_bots.go`) and `list_topics`/`get_topic` (`read_streams.go`) as-is. Do **not** add `list_tools`/`get_tool` (the catalogue is in the system prompt + MCP tool-list API).

## Remove Bot.Topics end-to-end (subscriptions are the source of truth)
- [x] Remove the `Topics` field and `WithTopics` from `orgchart.Bot`, and the `topics` parameter from `NewBot(...)`; update every call site (production + tests).
- [x] Drop `bots.UpdateParams.Topics` and its use in `Update`.
- [x] Remove the `Topics` field and mapping from the GORM `botRow` (+ memory store). This stops all reads/writes of the redundant `org_bots.topics` column (`org_subscriptions` is the source of truth).
- [x] Physical column: **left `org_bots.topics` orphaned** (AutoMigrate won't drop it; harmless unread column). No backfill into `org_subscriptions`.
- [x] Remove `Topics` from the read DTO (`read_bots.go`) and the REST DTOs (`dto.go`/`bots.go`); drop `toTopicIDs`/`toToolNames` helpers if now unused.

## Registration & authorization
- [x] In `builtins.go`, register `attach_tool`, `detach_tool`, `set_bot_content`, `delete_bot`; unregister `update_bot` and `invite_bots`.
- [x] In `defaults.go` `OwnerBotTools()`, replace `UpdateBotName`/`InviteBotsName` with `AttachToolName`, `DetachToolName`, `SetBotContentName`, `DeleteBotName`; keep `SubscribeName`/`UnsubscribeName`.

## Generated artifacts
- [x] Regenerate swagger + clients after removing `topics` from `BotDTO`/`UpdateBotRequest` (`swag init` → `docs.go`/`swagger.json`/`swagger.yaml`; copy to `frontend/swagger/` + root; `swagger2openapi` → `openapi.json`; `swagger-typescript-api` → `frontend/src/api/api.ts`). Diff scoped to the two removed `topics` fields.

## Tests & verification
- [ ] `attach_tool`/`detach_tool`: attach a multi-tool array; idempotent re-add; detach a subset; detach refuses if any name is baseline; unknown tool in the array rejected (no partial write); order-stable result.
- [ ] `subscribe`/`unsubscribe`: subscribe a bot to a topic array; self-subscribe; unsubscribe a subset; unknown bot or any unknown topic in the array rejected (no partial write); idempotent per topic.
- [ ] `create_bot`: `tools:["subscribe","dm"], topics:["t1","t2"]` (existing) → bot with tools ∪ baseline + a subscription row per topic; `tools:[], topics:[]` → tools == `BaseReadTools`, no subs; unknown topic → error and no bot row created; unknown tool name rejected; `tools` a required non-nullable enum array and `topics` a required non-nullable array.
- [ ] `set_bot_content`: content changes, tools preserved.
- [ ] `delete_bot`: deletes an existing bot (subscriptions + reporting lines gone, reports parentless); absent bot → not-found; `list_bots`/`get_bot`/`list_topics`/`get_topic` still work.
- [ ] Schema tests: `attach_tool.tools`/`detach_tool.tools` and `create_bot.tools` are required non-nullable arrays of enum items; `subscribe.topicIds`/`unsubscribe.topicIds` and `create_bot.topics` are required non-nullable string arrays; no `["null","array"]` union remains; registry additions appear in the enums.
- [x] Update `builtins_test.go` / `spec_tasks_registration_test.go`; fix `NewBot` call sites; `go build ./...` for the org packages.
- [ ] Manual MCP smoke (inner Helix): `create_topic` → `create_bot(topics:[…])`, publish to a listed topic and confirm the bot receives it → `attach_tool(botId, [subscribe, dm])` → `subscribe(botId, [topicId, …])` for later topics.
</content>
