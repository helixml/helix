# Replace bot-editing MCP tools with bulk attach/detach + subscribe; create_bot subscribes at creation

## Summary

The org MCP surface for editing bots was hard to use and partly broken:

- `create_bot`/`update_bot`/`invite_bots` took array arguments advertised with a
  nullable *union* JSON-Schema type (`"type":["null","array"]`), which small
  models mishandle — they send a bare string and the server rejects it
  (`cannot unmarshal string into []string`). This blocked granting tools.
- `create_bot`'s `topics` wrote a no-op manifest (`Bot.Topics`) that looked like
  it subscribed the bot but never created a subscription.

This reworks the surface around one principle (now recorded in `CLAUDE.md`):
**complete a user action in as few steps as possible.** Bulk, enum-validated,
non-nullable array arguments replace the broken ones, and creation does the whole
job — tools + subscriptions — in one call.

## New / changed MCP tools

| Tool | Args | Notes |
|---|---|---|
| `create_bot` | `id?`, `content`, `tools` (enum array), `topics` (array), `parentId?` | grants tools (∪ read baseline) and **subscribes** to `topics` at creation |
| `attach_tool` / `detach_tool` | `botId`, `tools` (enum array) | bulk grant/revoke; `detach` refuses baseline tools |
| `subscribe` / `unsubscribe` | `botId`, `topicIds` (array) | target any bot (was caller-only); bulk |
| `set_bot_content` | `botId`, `content` | replaces the content half of the removed `update_bot` |
| `delete_bot` | `botId` | wraps the existing `lifecycle.Delete` cascade |

Removed: `update_bot`, `invite_bots`, and the caller-only `subscribe`/`unsubscribe`.
`list_bots`/`get_bot`/`list_topics`/`get_topic` are unchanged (existing baseline
reads); no `list_tools`/`get_tool` (the catalogue is already in the system prompt
+ MCP tool-list API).

## Key changes

- **Schema fix:** `tools`/`topics`/`topicIds` are non-nullable arrays (no null
  union). Tool arguments carry a JSON-Schema `enum` built dynamically from the
  live registry (`Deps.ToolNames`, wired in `RegisterBuiltins`), so valid names
  are discoverable and new tools appear automatically.
- **create_bot subscribes:** `lifecycle.Create` validates every topic exists
  **before** writing the bot row (no partial creates), then reuses the same
  `subscriptions.SubscribeTopics` use case the `subscribe` tool drives (DRY).
  Wired for both MCP and REST create paths.
- **`Bot.Topics` removed end-to-end** (domain, `NewBot`, GORM `botRow`, read DTO,
  REST DTOs). Subscriptions live only as `org_subscriptions` rows — the single
  source of truth. The physical `org_bots.topics` column is left orphaned
  (GORM AutoMigrate won't drop it; harmless). No backfill of the old no-op
  manifest into real subscriptions.
- New service methods: `bots.AttachTools`/`DetachTools`,
  `subscriptions.SubscribeTopics`/`UnsubscribeTopics`.
- Regenerated swagger/OpenAPI/TS client (only the two removed `topics` fields).

## Testing

- `go build ./...` and `go test ./pkg/org/...` — all pass (org tests use the
  in-memory store; no Postgres needed).
- New `bulk_tools_test.go`: attach/detach (multi-tool, idempotent, baseline
  refusal, unknown-tool rejection), create-time subscription (+ unknown-topic
  leaves no partial bot), `delete_bot` cascade, and schema shape (non-nullable
  enum arrays, registry-derived enum).
- Updated integration tests drive the flow over a real MCP httptest server
  (`TestDemoOwnerCreatesCEO`, `TestSubscribeOtherBots`).
- NOT run: live inner-Helix browser/agent smoke.
