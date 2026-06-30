# Implementation Tasks: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Change A — Accept string-or-array on array arguments
- [ ] Add a reusable `stringList` type with `UnmarshalJSON` accepting a bare string or an array of strings (empty string → empty list) in `interfaces/mcptools/schema.go`.
- [ ] Add a `stringOrStringArray(description)` schema helper that builds a `oneOf: [string, array<string>]` property (mirror the `transport` override in `create_stream.go`).
- [ ] Use `stringList` for `createBotArgs.Tools`, convert to `[]tool.Name` when building params, and override the `tools` schema (`create_bot.go`).
- [ ] Use `stringList` for `updateBotArgs.Tools`, override the `tools` schema, and preserve nil-vs-empty semantics (omitted = preserve, `[]` = clear) (`update_bot.go`).
- [ ] Use `stringList` for `inviteBotsArgs.BotIDs` and override the `botIds` schema (`invite_bots.go`).

## Change B — Remove streams/topics from bot creation (dead no-op)
- [ ] Drop the `Topics` field from `createBotArgs`; `create_bot` args become `id`/`content`/`tools`/`parentId` (`interfaces/mcptools/create_bot.go`).
- [ ] Update `CreateBot.Description()` to remove topics/streams wording and point to the follow-up (`create_topic` + `invite_bots`, or `subscribe`).
- [ ] Remove the `Topics` field and `WithTopics` from `orgchart.Bot`, and remove the `topics` parameter from `NewBot(...)` (`domain/orgchart/bot.go`).
- [ ] Update every `NewBot(...)` call site (production + all tests) to the new signature.
- [ ] Drop `Topics` from `lifecycle.CreateParams`, `bots.CreateParams`, and `bots.UpdateParams` and their uses; keep `bots.Create` applying `MergeTools(p.Tools, baseTools)` (`application/lifecycle/lifecycle.go`, `application/bots/bots.go`).
- [ ] Remove the `Topics` column/field and mapping from the GORM bot model and the memory store. Note: leftover DB column is harmless; no migration needed (`infrastructure/persistence/gorm/bot.go`).
- [ ] Remove `Topics` from the read DTO (`interfaces/mcptools/read_bots.go`) and from the REST create/update request/response DTOs and the `toTopicIDs` helper if now unused (`interfaces/server/api/dto.go`, `interfaces/server/api/bots.go`).

## Tests & verification
- [ ] `create_bot` tests: `"tools":"subscribe"` and `"tools":["subscribe","dm"]` both create a bot whose tools union with `BaseReadTools` identically; `topics` is no longer a schema field.
- [ ] `update_bot` tests: grant `subscribe`/`dm` via bare string and via array; omitted `tools` preserves; `[]` clears.
- [ ] `invite_bots` tests: single-string and array `botIds` both work.
- [ ] Schema/contract tests (`schema_test.go`, `builtins_test.go`): `tools`/`botIds` advertise the `oneOf` shape; no `["null","array"]` union remains; account for the `topics` removal.
- [ ] `go build ./...` for the org packages; fix any compile errors from the `NewBot` signature change.
- [ ] Manual MCP smoke in inner Helix: `create_bot` granting `subscribe`/`dm` via a bare string → `create_topic` → `invite_bots` to subscribe the new bot (the original failing flow).
</content>
