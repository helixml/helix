# Design: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Where the code lives

All in `api/pkg/org/`:

- MCP tools: `interfaces/mcptools/create_bot.go`, `update_bot.go`, `invite_bots.go`,
  `subscribe.go`, `schema.go` (schema helpers), `create_topic.go` (the precedent).
- REST + DTO: `interfaces/server/api/bots.go`.
- Application: `application/lifecycle/lifecycle.go`, `application/bots/bots.go`.
- Domain: `domain/orgchart/bot.go` (`NewBot`, `Bot.Topics`, `WithTopics`).
- Persistence: `infrastructure/persistence/gorm/bot.go` (and the memory store).
- Types: `domain/tool/tool.go` (`type Name = string`), `domain/streaming/ids.go`
  (`type TopicID = string`) — both are **aliases for `string`**, so `[]tool.Name` is
  literally `[]string`.

## Change A — Remove the `topics` manifest (decouple subscription from creation)

`Bot.Topics` is functionless: it is stored verbatim and never produces a subscription
(confirmed by the domain comment and by tracing `lifecycle.Create` → `bots.Create` —
nothing in the create path subscribes based on it). Leaving a no-op field that *looks*
like it subscribes is the root cause of the user's confusion, so we remove it outright
rather than just hiding it from `create_bot` (a half-measure that keeps the trap reachable
via REST and the response DTO).

Remove `topics`/`Topics` end-to-end:
- `createBotArgs` — drop the `Topics` field; remove all topic wording from
  `CreateBot.Description()`; redirect callers to `invite_bots`/`subscribe` as the
  follow-up.
- `lifecycle.CreateParams` and `bots.CreateParams`/`bots.UpdateParams` — drop `Topics`.
- `orgchart`: drop the `Topics` field from `Bot`, drop `WithTopics`, and remove the
  `topics` parameter from `NewBot(...)`. This is the only signature change with reach —
  update every `NewBot` call site (production + tests) accordingly.
- `infrastructure/persistence/gorm/bot.go` and the memory store — drop the `Topics`
  column/field from the row model and its mapping. (GORM AutoMigrate does not drop the
  existing DB column; an orphaned, unread column is harmless — note it, no migration
  needed.)
- REST `bots.go` — drop `Topics` from the create request, update request, and response
  DTOs, and the `toTopicIDs` helper if it becomes unused.

After this, subscription is exclusively explicit and post-creation: `subscribe` (caller)
and `invite_bots` (other bots) — no behaviour change to those tools.

## Change B — Accept string-or-array for array arguments

### Root cause
The advertised schema is *correct* (array): `jsonschema.For[[]string]` emits
`{"type":["null","array"],"items":{"type":"string"}}`. The real failure is that small
chat models routinely collapse a one-element array to a bare string
(`"tools":"subscribe"`), which then fails `json.Unmarshal` into `[]string`. This is the
exact failure mode already solved for `create_topic`'s `transport` field
(`createTopicTransport.UnmarshalJSON` accepts a bare string shorthand, and the schema is
overridden to a `oneOf`). We mirror that precedent.

### Lenient unmarshalling
Add a small reusable type in `mcptools` (e.g. `schema.go`):

```go
// stringList accepts either a JSON array of strings (["a","b"]) or a single
// bare string ("a" -> ["a"]). Empty string -> empty list. Mirrors the
// string-shorthand handling on create_topic's transport.
type stringList []string

func (l *stringList) UnmarshalJSON(data []byte) error {
    if len(data) > 0 && data[0] == '"' {        // bare string shorthand
        var s string
        if err := json.Unmarshal(data, &s); err != nil { return err }
        if s == "" { *l = stringList{} } else { *l = stringList{s} }
        return nil
    }
    var arr []string
    if err := json.Unmarshal(data, &arr); err != nil { return err }
    *l = arr
    return nil
}
```

Use `stringList` for:
- `createBotArgs.Tools` and `updateBotArgs.Tools` (convert to `[]tool.Name` when calling
  the service — trivial since `tool.Name == string`).
- `inviteBotsArgs.BotIDs` (same failure mode on the follow-up subscribe step).

`update_bot`'s nil-vs-empty semantics are preserved: an absent `tools` key never invokes
`UnmarshalJSON`, so the field stays `nil` → "preserve"; `[]` or `""` → empty → "clear".

### Schema override
For each affected tool, override the advertised property schema (same approach as
`createTopicSchema`) so strict clients accept both shapes:

```json
"tools": {
  "description": "MCP tools to grant. Either a single tool name as a string, or an array of tool names.",
  "oneOf": [
    { "type": "string" },
    { "type": "array", "items": { "type": "string" } }
  ]
}
```

Add a small helper in `schema.go` (e.g. `stringOrStringArray(description)`) to build this,
and apply it to the `tools` property of `create_bot`/`update_bot` and the `botIds` property
of `invite_bots`, the way `create_topic` rewrites its `transport` property.

## Key decisions
- **Full removal of `Topics`, not just hiding it.** It is dead, misleading code; the repo
  rule is to remove dead code. The cost is a mechanical `NewBot` signature change.
- **String-or-array via custom unmarshal + `oneOf` schema**, reusing the established
  `create_topic` pattern rather than inventing a new convention. No change to
  `jsonschema.For` / `mustSchema` machinery.
- **No new MCP tool.** `subscribe` + `invite_bots` already implement the follow-up; the
  blocker was the schema bug, now fixed.

## Testing
- Unit (`create_bot_test.go`, `update_bot` tests, `invite_bots` tests, `builtins_test.go`):
  - `"tools":"subscribe"` and `"tools":["publish","subscribe"]` both succeed and union
    with `BaseReadTools` identically (extend `TestCreateBotUnionWithCallerTools`).
  - `update_bot` with omitted `tools` preserves; with `[]` clears.
  - `create_bot` rejects/ignores a `topics` key (it is no longer in the schema); creation
    still succeeds.
  - `invite_bots` accepts a single `botIds` string and an array.
- `go build ./...` for the org packages; fix all `NewBot` call sites.
- Manual MCP smoke (inner Helix): create a bot, `update_bot` to grant `subscribe`/`dm`
  with a bare string, then `invite_bots` to subscribe it to a topic — the original failing
  flow.
</content>
