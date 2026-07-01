# Implementation Tasks: Replace Bot-Editing MCP Tools with Discrete Attach/Detach and Subscribe Operations

## New discrete tool grants (attach_tool / detach_tool)
- [ ] Add `AttachTool(ctx, orgID, id, name)` and `DetachTool(ctx, orgID, id, name)` to `application/bots/bots.go` (read-modify-write via the existing `Update` path; `DetachTool` refuses to remove a `BaseReadTools` entry; both idempotent).
- [ ] Add a names provider to the mcptools `Deps` (e.g. `ToolNames func() []tool.Name`) and wire it in `RegisterBuiltins` to return `reg.List()` names.
- [ ] Add a `schema.go` helper (e.g. `enumStringProperty(names, description)`) that builds a required, non-nullable string `enum` property.
- [ ] Create `attach_tool.go` (`attach_tool`, args `{botId, tool}`) building its schema from the names provider in `InputSchema()`; validate the tool name; call `bots.AttachTool`.
- [ ] Create `detach_tool.go` (`detach_tool`, args `{botId, tool}`) mirroring attach; call `bots.DetachTool`.

## Bot-targeted subscribe / unsubscribe
- [ ] Change `subscribe.go` args to `{botId, topicId}` and pass `botId` as the worker id to `subscriptions.Subscribe`; update description.
- [ ] Change `unsubscribe.go` args to `{botId, topicId}` and pass `botId` to `subscriptions.Unsubscribe`; update description.
- [ ] Delete `invite_bots.go` and `InviteBotsName`; remove all references.

## Bare create_bot; remove update_bot; content editor
- [ ] Drop `Tools` and `Topics` from `createBotArgs`; `create_bot` args become `id`/`content`/`parentId`; update `CreateBot.Description()` to point at `attach_tool`/`subscribe`/`set_bot_content` (`create_bot.go`).
- [ ] Delete `update_bot.go` and `UpdateBotName`; remove all references.
- [ ] Create `set_bot_content.go` (`set_bot_content`, args `{botId, content}`) calling `bots.Update` with only `Content` set. *(Drop this task if content is meant to be immutable.)*
- [ ] Drop `Tools` (and `Topics`) from `bots.CreateParams` and `lifecycle.CreateParams`; `bots.Create` applies `MergeTools(nil, baseTools)`.

## Remove Bot.Topics end-to-end
- [ ] Remove the `Topics` field and `WithTopics` from `orgchart.Bot`, and the `topics` parameter from `NewBot(...)`; update every call site (production + tests).
- [ ] Drop `Topics` from `bots.UpdateParams` and its use in `Update`.
- [ ] Remove the `Topics` column/field and mapping from the GORM bot model and the memory store (leftover DB column is harmless; no migration).
- [ ] Remove `Topics` from the read DTO (`read_bots.go`) and the REST DTOs (`dto.go`/`bots.go`); drop `toTopicIDs`/`toToolNames` helpers if now unused.

## Registration & authorization
- [ ] In `builtins.go`, register `attach_tool`, `detach_tool`, `set_bot_content`; unregister `update_bot` and `invite_bots`.
- [ ] In `defaults.go` `OwnerBotTools()`, replace `UpdateBotName`/`InviteBotsName` with `AttachToolName`, `DetachToolName`, `SetBotContentName`; keep `SubscribeName`/`UnsubscribeName`.

## Tests & verification
- [ ] `attach_tool`/`detach_tool` tests: add; idempotent re-add; detach non-baseline; detach refuses baseline; unknown tool rejected; order-stable result.
- [ ] `subscribe`/`unsubscribe` tests: subscribe another bot; self-subscribe; unsubscribe; unknown bot/topic rejected.
- [ ] `create_bot` test: content-only create → tools == `BaseReadTools`; no `tools`/`topics` schema fields.
- [ ] `set_bot_content` test: content changes, tools preserved.
- [ ] Schema tests: `attach_tool.tool` is a non-nullable `enum`; registry additions appear in the enum; no `["null","array"]` union remains.
- [ ] Update `builtins_test.go` / `spec_tasks_registration_test.go` for the registered/unregistered set; fix `NewBot` call sites; `go build ./...` for the org packages.
- [ ] Manual MCP smoke (inner Helix): `create_bot` → `attach_tool` (`subscribe`/`dm`) → `create_topic` → `subscribe(botId, topicId)`.
</content>
