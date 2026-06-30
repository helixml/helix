# Design: Decouple Stream Subscription from Bot Creation and Fix Array-Argument Schema

## Where the code lives

All in `api/pkg/org/`:

- MCP tools: `interfaces/mcptools/create_bot.go`, `update_bot.go`,
  `invite_bots.go`, `subscribe.go`, `schema.go` (schema helpers), and
  `create_stream.go` (the tool named `create_topic` — the precedent for the
  string-or-object fix).
- REST + DTO: `interfaces/server/api/dto.go`, `interfaces/server/api/bots.go`.
- Application: `application/lifecycle/lifecycle.go`, `application/bots/bots.go`.
- Domain: `domain/orgchart/bot.go` (`Bot.Topics`, `NewBot`, `WithTopics`).
- Persistence: `infrastructure/persistence/gorm/bot.go` (+ memory store if any).
- Read DTO: `interfaces/mcptools/read_bots.go`.
- Types: `domain/tool/tool.go` (`type Name = string`) and
  `domain/streaming/ids.go` (`type TopicID = string`) are **aliases for
  `string`**, so `[]tool.Name` is literally `[]string`.

## Change A — Accept string-or-array on the array arguments

### Root cause (verified)
The auto-generated schema for a `[]string` field is a *nullable union*, not a
plain array. Dumping `createBotSchema` confirms:

```json
"tools": { "type": ["null","array"], "items": { "type": "string" } }
```

Scalar fields (`id`, `content`) get a clean `"type":"string"` and work fine.
Function-calling schema layers that don't accept a union/array-valued `type`
treat the field as scalar, so the model emits `"tools":"subscribe"`, and the
server's `json.Unmarshal` into `[]string` fails.

This is the exact failure already handled for `create_topic`'s `transport`
field in `create_stream.go`: its `createTopicTransport.UnmarshalJSON` accepts a
bare-string shorthand, and `createTopicSchema` rewrites the property to a
`oneOf` (also stripping the `["object","null"]` union — note the comment there:
"`Type+Types together is a marshal error`"). We mirror that precedent exactly.

Applies to: `create_bot.tools`, `update_bot.tools`, `invite_bots.botIds`.

### Lenient unmarshalling
Add one small reusable type in `mcptools` (e.g. `schema.go`):

```go
// stringList accepts either a JSON array of strings (["a","b"]) or a single
// bare string ("a" -> ["a"]). Empty string -> empty list. Mirrors the
// string-shorthand handling on create_topic's transport field.
type stringList []string

func (l *stringList) UnmarshalJSON(data []byte) error {
    if len(data) > 0 && data[0] == '"' {        // bare-string shorthand
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

Use `stringList` for `createBotArgs.Tools`, `updateBotArgs.Tools`, and
`inviteBotsArgs.BotIDs`. Convert to `[]tool.Name` / `[]orgchart.BotID` when
calling the service (trivial — the element types are `string` aliases).

`update_bot`'s nil-vs-empty semantics are preserved: an absent `tools` key
never invokes `UnmarshalJSON`, so the field stays `nil` → "preserve"; `[]` or
`""` → empty → "clear".

### Schema override
Add a helper in `schema.go` (e.g. `stringOrStringArray(description)`) that
builds:

```json
{
  "description": "...",
  "oneOf": [
    { "type": "string" },
    { "type": "array", "items": { "type": "string" } }
  ]
}
```

Apply it (the way `createTopicSchema` rewrites `transport`) to:
`create_bot`'s `tools`, `update_bot`'s `tools`, and `invite_bots`'s `botIds`.

## Change B — Bot creation does not take streams (remove `topics`)

`create_bot`'s `topics` argument feeds `Bot.Topics`, a no-op manifest that is
stored verbatim and never produces a subscription (confirmed by the domain
comment on `bot.go` and by tracing `lifecycle.Create` → `bots.Create`). Per
CLAUDE.md ("clean up dead code") and the helix-org philosophy (streams stay
prompt-driven, subscription is explicit), remove it end-to-end rather than just
hiding it from the schema:

- `interfaces/mcptools/create_bot.go` — drop the `Topics` field from
  `createBotArgs`; update `CreateBot.Description()` to drop topics/streams
  wording and point to the follow-up (`create_topic` + `invite_bots`, or the
  Bot self-subscribes via `subscribe`). **Keep the `Tools` field** (now a
  `stringList`, per Change A).
- `domain/orgchart/bot.go` — drop the `Topics` field and `WithTopics`, and
  remove the `topics` parameter from `NewBot(...)`.
- `application/lifecycle/lifecycle.go` + `application/bots/bots.go` — drop
  `Topics` from `CreateParams`/`UpdateParams` and their uses. `bots.Create`
  keeps building the bot with `MergeTools(p.Tools, baseTools)` so new bots
  still get the baseline unioned with any supplied tools.
- `infrastructure/persistence/gorm/bot.go` (+ memory store) — drop the
  `Topics` column/field and its marshal/unmarshal. GORM AutoMigrate does not
  drop the existing DB column; an orphaned unread column is harmless — note it,
  no migration needed.
- `interfaces/mcptools/read_bots.go` + REST `dto.go`/`bots.go` — drop `Topics`
  from the read DTO and from the create/update request DTOs, and the
  `toTopicIDs` helper if now unused.

Net: `create_bot` (MCP + REST) takes `id`/`content`/`tools`/`parentId`; no
topics anywhere on the bot create/update path; subscription is only ever the
explicit (worker, topic) link created by `subscribe` / `invite_bots`.

## Change C — The follow-up subscription path (no code, documentation only)

`invite_bots` already subscribes other Bots to a Topic (`subscriptions.Invite`
validates the Topic and each Bot exist; no ownership check). The intended flow
becomes:

1. `create_bot` (content + tools, including `subscribe`/`dm` if wanted)
2. `create_topic` (if the Topic doesn't exist yet)
3. `invite_bots` (manager subscribes the new Bot) — or the Bot calls
   `subscribe` itself once it holds the tool.

Make this explicit in the `create_bot` description so the model reaches for the
follow-up instead of expecting creation to subscribe.

## Key decisions
- **Fix the array bug by reusing the `create_topic` precedent** (custom
  `UnmarshalJSON` + `oneOf` schema), not by changing the `jsonschema.For` /
  `mustSchema` machinery. One new `stringList` type + one schema helper, applied
  to three properties.
- **Keep `tools` on `create_bot`.** Tools are part of a Bot's definition (the
  Role *is* the capability); once the array bug is fixed, granting tools at
  creation works in one call. The user asked to decouple *streams*, not tools.
- **Remove `topics`/`Bot.Topics` fully** — it is a misleading no-op, so it is
  dead code, not a feature to relocate.
- **No new MCP tool** — `invite_bots` / `subscribe` already cover the follow-up.

## Testing
- Unit (`create_bot_test.go`, `update_bot` tests, `invite_bots` tests,
  `schema_test.go`, `builtins_test.go`):
  - `create_bot` with `"tools":"subscribe"` and `"tools":["subscribe","dm"]`
    both succeed and union with `BaseReadTools` identically; a `topics` key is
    no longer part of the schema.
  - `update_bot`: bare-string and array `tools` both work; omitted preserves;
    `[]` clears.
  - `invite_bots`: single-string and array `botIds` both work.
  - Schema assertions: `tools`/`botIds` advertise the `oneOf` shape; no
    `["null","array"]` union remains.
- `go build ./...` for the org packages; fix all `NewBot` call sites from the
  signature change.
- Manual MCP smoke (inner Helix): the original failing flow —
  `create_bot` granting `subscribe`/`dm` via a bare string → `create_topic` →
  `invite_bots` to subscribe the new Bot.
</content>
