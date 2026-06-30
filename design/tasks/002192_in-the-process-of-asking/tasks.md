# Implementation Tasks: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Change A — Remove the `topics` manifest
- [ ] Drop the `Topics` field from `createBotArgs` and remove all topic/stream wording from `CreateBot.Description()`; redirect callers to `invite_bots`/`subscribe` as the explicit follow-up (`interfaces/mcptools/create_bot.go`).
- [ ] Drop `Topics` from `lifecycle.CreateParams` and stop passing it through `Create` (`application/lifecycle/lifecycle.go`).
- [ ] Drop `Topics` from `bots.CreateParams` and `bots.UpdateParams` and their use in `bots.Create`/`Update` (`application/bots/bots.go`).
- [ ] Remove the `Topics` field and `WithTopics` from `orgchart.Bot`, and remove the `topics` parameter from `NewBot(...)` (`domain/orgchart/bot.go`).
- [ ] Update every `NewBot(...)` call site (production + all tests) to the new signature.
- [ ] Remove the `Topics` column/field and mapping from the GORM bot model and the memory store (`infrastructure/persistence/gorm/bot.go` + memory store). Note: leftover DB column is harmless; no migration needed.
- [ ] Remove `Topics` from the REST create request, update request, and response DTOs and the `toTopicIDs` helper if now unused (`interfaces/server/api/bots.go`).

## Change B — Accept string-or-array for array arguments
- [ ] Add a reusable `stringList` type with `UnmarshalJSON` accepting a bare string or an array of strings (empty string → empty list) in `interfaces/mcptools/schema.go`.
- [ ] Add a `stringOrStringArray(description)` schema helper that builds a `oneOf: [string, array<string>]` property (mirror the `create_topic` transport schema override).
- [ ] Use `stringList` for `createBotArgs.Tools` and override the advertised `tools` schema; keep the base-read-tool union behaviour (`create_bot.go`).
- [ ] Use `stringList` for `updateBotArgs.Tools`, override the `tools` schema, and preserve nil-vs-empty semantics (omitted = preserve, `[]` = clear) (`update_bot.go`).
- [ ] Use `stringList` for `inviteBotsArgs.BotIDs` and override the `botIds` schema (`invite_bots.go`).

## Tests & verification
- [ ] Extend `create_bot_test.go`: bare-string `tools` and array `tools` both union correctly with `BaseReadTools`; a `topics` key is no longer accepted/used; creation still succeeds.
- [ ] Add/extend `update_bot` tests: grant `subscribe`/`dm` via bare string; omitted `tools` preserves; `[]` clears.
- [ ] Add/extend `invite_bots` tests: single-string and array `botIds` both work.
- [ ] Update `builtins_test.go` and any schema/contract tests affected by the `topics` removal and `tools`/`botIds` schema change.
- [ ] `go build ./...` for the org packages; fix any remaining compile errors from the `NewBot` signature change.
- [ ] Manual MCP smoke in inner Helix: create a bot → `update_bot` to grant `subscribe`/`dm` with a bare string → `invite_bots` to subscribe it to a topic (the original failing flow).
</content>
