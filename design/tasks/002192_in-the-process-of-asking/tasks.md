# Implementation Tasks: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Change A — Creation does one thing (remove `tools` and `topics` from create)

### Remove `topics` entirely (dead no-op)
- [ ] Remove the `Topics` field and `WithTopics` from `orgchart.Bot`, and remove the `topics` parameter from `NewBot(...)` (`domain/orgchart/bot.go`).
- [ ] Update every `NewBot(...)` call site (production + all tests) to the new signature.
- [ ] Drop `Topics` from `lifecycle.CreateParams`, `bots.CreateParams`, and `bots.UpdateParams` and their uses (`application/lifecycle/lifecycle.go`, `application/bots/bots.go`).
- [ ] Remove the `Topics` column/field and mapping from the GORM bot model and the memory store. Note: leftover DB column is harmless; no migration needed.
- [ ] Remove `Topics` from the REST create/update request and response DTOs and the `toTopicIDs` helper if now unused (`interfaces/server/api/bots.go`).

### Remove `tools` from the create path (assign via `update_bot`)
- [ ] Drop the `Tools` field from `createBotArgs`; `create_bot` args become `id`/`content`/`parentId` only (`interfaces/mcptools/create_bot.go`).
- [ ] Update `CreateBot.Description()` to remove tools/topics wording and point to `update_bot` (grant tools) and `invite_bots`/`subscribe` (subscribe).
- [ ] Drop `Tools` from `lifecycle.CreateParams` and `bots.CreateParams`; have `bots.Create` always apply the baseline (`MergeTools(nil, baseTools)`) so new bots still get `BaseReadTools`.
- [ ] Drop `Tools` from the REST `CreateBotRequest` (the UI never sent it); keep `update_bot`'s `Tools` as the single tool-assignment path.

## Change B — Accept string-or-array on the surviving array arguments
- [ ] Add a reusable `stringList` type with `UnmarshalJSON` accepting a bare string or an array of strings (empty string → empty list) in `interfaces/mcptools/schema.go`.
- [ ] Add a `stringOrStringArray(description)` schema helper that builds a `oneOf: [string, array<string>]` property (mirror the `create_topic` transport schema override).
- [ ] Use `stringList` for `updateBotArgs.Tools`, override the `tools` schema, and preserve nil-vs-empty semantics (omitted = preserve, `[]` = clear) (`update_bot.go`).
- [ ] Use `stringList` for `inviteBotsArgs.BotIDs` and override the `botIds` schema (`invite_bots.go`).

## Tests & verification
- [ ] Update `create_bot_test.go`: a content-only `create_bot` (optional `id`/`parentId`) yields tools == `BaseReadTools`; `tools`/`topics` are no longer schema fields. Replace `TestCreateBotUnionWithCallerTools` (create-time tools no longer exist).
- [ ] Add/extend `update_bot` tests: grant `subscribe`/`dm` via bare string and via array; omitted `tools` preserves; `[]` clears.
- [ ] Add/extend `invite_bots` tests: single-string and array `botIds` both work.
- [ ] Update `builtins_test.go` and any schema/contract tests affected by the `topics` removal, the `create_bot` tools removal, and the `tools`/`botIds` schema change.
- [ ] `go build ./...` for the org packages; fix any remaining compile errors from the `NewBot` signature change.
- [ ] Manual MCP smoke in inner Helix: `create_bot` (content only) → `update_bot` to grant `subscribe`/`dm` with a bare string → `invite_bots` to subscribe it to a topic (the original failing flow).
</content>
