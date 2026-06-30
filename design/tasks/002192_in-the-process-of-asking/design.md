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

## Change A — Creation does one thing (remove `tools` and `topics` from create)

The guiding pattern (per review): `create_bot` creates the bot from its markdown; tools
and subscriptions are assigned **after** creation with the existing edit tools. The chart
UI already works this way (`NewBotDialog` posts only `id`/`content`/`parent_id`), so this
aligns the MCP + REST create surface with what the UI already does.

### Remove `topics` entirely (it is dead, misleading code)
`Bot.Topics` is functionless: stored verbatim, never producing a subscription (confirmed
by the domain comment and by tracing `lifecycle.Create` → `bots.Create`). Remove it
end-to-end rather than just hiding it:
- `domain/orgchart/bot.go` — drop the `Topics` field and `WithTopics`, and remove the
  `topics` parameter from `NewBot(...)`.
- `application/lifecycle` + `application/bots` — drop `Topics` from `CreateParams`/
  `UpdateParams` and their use.
- `infrastructure/persistence/gorm/bot.go` + memory store — drop the `Topics`
  column/field and mapping. (GORM AutoMigrate does not drop the existing DB column; an
  orphaned, unread column is harmless — note it, no migration needed.)
- REST `bots.go` — drop `Topics` from the create/update request and response DTOs and the
  `toTopicIDs` helper if now unused.

### Remove `tools` from the create path (assign via `update_bot` instead)
Unlike `Topics`, `Bot.Tools` is real and stays — we only stop accepting it at creation:
- `createBotArgs` — drop the `Tools` field; `create_bot` args become `id`, `content`,
  `parentId` only. Update `CreateBot.Description()` to drop tools/topics wording and point
  to `update_bot` (grant tools) and `invite_bots`/`subscribe` (subscribe).
- `application/lifecycle.CreateParams` and `application/bots.CreateParams` — drop `Tools`.
  `bots.Create` always constructs the bot with the baseline only
  (`MergeTools(nil, baseTools)` == `BaseReadTools`), so every new bot still gets a usable
  read surface. Granting more is `update_bot`'s job.
- REST `CreateBotRequest` — drop `Tools` (the UI never sent it). Keep `update_bot`'s
  `Tools` as the single place tools are assigned/edited.

Net result: `create_bot` (MCP + REST) takes only `id`/`content`/`parentId`; the only
remaining array argument on the bot tools is `update_bot.tools`.

## Change B — Accept string-or-array on the surviving array arguments

### Root cause
The advertised schema is *correct* (array): `jsonschema.For[[]string]` emits
`{"type":["null","array"],"items":{"type":"string"}}`. The real failure is that small chat
models routinely collapse a one-element array to a bare string (`"tools":"subscribe"`),
which then fails `json.Unmarshal` into `[]string`. This is the exact failure mode already
solved for `create_topic`'s `transport` field (`createTopicTransport.UnmarshalJSON`
accepts a bare-string shorthand, and the schema is overridden to a `oneOf`). We mirror that
precedent.

Applies to the two surviving array args: `update_bot.tools` and `invite_bots.botIds`.
(`create_bot` no longer has any array argument after Change A.)

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
- `updateBotArgs.Tools` (convert to `[]tool.Name` when calling the service — trivial since
  `tool.Name == string`).
- `inviteBotsArgs.BotIDs`.

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
and apply it to the `tools` property of `update_bot` and the `botIds` property of
`invite_bots`, the way `create_topic` rewrites its `transport` property.

## Key decisions
- **Creation is content-only** (`id`/`content`/`parentId`). Tools → `update_bot`,
  subscriptions → `invite_bots`/`subscribe`. This matches the review guidance and the
  existing chart UI, and shrinks the create surface to scalars.
- **Full removal of `Topics`** (dead, misleading no-op), at the cost of a mechanical
  `NewBot` signature change.
- **String-or-array via custom unmarshal + `oneOf` schema**, reusing the established
  `create_topic` pattern rather than inventing a new convention. No change to
  `jsonschema.For` / `mustSchema` machinery.
- **No new MCP tool.** `update_bot` + `subscribe` + `invite_bots` already implement the
  follow-ups; the blocker was the schema bug, now fixed.

## Testing
- Unit (`create_bot_test.go`, `update_bot` tests, `invite_bots` tests, `builtins_test.go`):
  - `create_bot` with only `content` (and optional `id`/`parentId`) creates a bot whose
    tools are exactly `BaseReadTools`; a `tools` or `topics` key is no longer part of the
    schema. (Replaces `TestCreateBotUnionWithCallerTools`, which tested create-time tools.)
  - `update_bot`: `"tools":"subscribe"` and `"tools":["publish","subscribe"]` both union
    with `BaseReadTools` identically; omitted `tools` preserves; `[]` clears.
  - `invite_bots`: single-string and array `botIds` both work.
- `go build ./...` for the org packages; fix all `NewBot` call sites from the signature
  change.
- Manual MCP smoke (inner Helix): `create_bot` (content only) → `update_bot` to grant
  `subscribe`/`dm` with a bare string → `invite_bots` to subscribe it to a topic — the
  original failing flow.
</content>
