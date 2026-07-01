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

## create_bot keeps topics (fixed schema); remove update_bot; content editor
- [ ] Drop `Tools` from `createBotArgs` but **keep `Topics`**; `create_bot` args become `id`/`content`/`topics`/`parentId`; update `CreateBot.Description()` to point at `attach_tool`/`subscribe`/`set_bot_content` and note `topics` is a declarative manifest, not a subscription (`create_bot.go`).
- [ ] Add a `schema.go` helper (e.g. `stringArrayProperty(description)`) for a non-nullable `{"type":"array","items":{"type":"string"}}` property.
- [ ] Override `create_bot`'s `topics` property with that helper and add `topics` to the schema's `required`; drop `omitempty` on the Go field (`create_bot.go`).
- [ ] Delete `update_bot.go` and `UpdateBotName`; remove all references.
- [ ] Create `set_bot_content.go` (`set_bot_content`, args `{botId, content}`) calling `bots.Update` with only `Content` set. *(Drop this task if content is meant to be immutable.)*
- [ ] Drop `Tools` from `bots.CreateParams` and `lifecycle.CreateParams`; keep `Topics`; `bots.Create` applies `MergeTools(nil, baseTools)` plus the topics manifest.

## Keep Bot.Topics
- [ ] Leave `Bot.Topics`, `WithTopics`, the `NewBot` topics param, `bots.UpdateParams.Topics`, the GORM column/mapping, the read DTO, and the REST DTOs unchanged. (No end-to-end removal — reversed by decision.)

## Registration & authorization
- [ ] In `builtins.go`, register `attach_tool`, `detach_tool`, `set_bot_content`; unregister `update_bot` and `invite_bots`.
- [ ] In `defaults.go` `OwnerBotTools()`, replace `UpdateBotName`/`InviteBotsName` with `AttachToolName`, `DetachToolName`, `SetBotContentName`; keep `SubscribeName`/`UnsubscribeName`.

## Tests & verification
- [ ] `attach_tool`/`detach_tool` tests: add; idempotent re-add; detach non-baseline; detach refuses baseline; unknown tool rejected; order-stable result.
- [ ] `subscribe`/`unsubscribe` tests: subscribe another bot; self-subscribe; unsubscribe; unknown bot/topic rejected.
- [ ] `create_bot` test: create with `topics: []` → tools == `BaseReadTools`, empty manifest; `topics: ["t1"]` stored; no `tools` schema field.
- [ ] `set_bot_content` test: content changes, tools and topics preserved.
- [ ] Schema tests: `attach_tool.tool` is a non-nullable `enum`; `create_bot.topics` is a non-nullable, required array (no `["null","array"]` union); registry additions appear in the enum.
- [ ] Update `builtins_test.go` / `spec_tasks_registration_test.go` for the registered/unregistered set; fix `NewBot` call sites; `go build ./...` for the org packages.
- [ ] Manual MCP smoke (inner Helix): `create_bot` → `attach_tool` (`subscribe`/`dm`) → `create_topic` → `subscribe(botId, topicId)`.
</content>
