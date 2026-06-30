# Add per-bot toggle to preserve conversation context across triggers

## Summary

Slack-support experiment for helix-org. Adds an opt-in per-bot setting,
`PreserveContext` (default **off**), that stops the runtime spawner from
wiping a bot's chat session before each re-activation. With it on, a bot
keeps its conversation (and, for Zed/ACP, the same thread) across triggers,
so Slack replies are faster and more context-aware instead of starting cold
every time. Default-off preserves today's wipe-on-every-trigger behaviour
for every existing bot.

## Changes

- **Domain** (`orgchart.Bot`): new `PreserveContext bool` field +
  `WithPreserveContext` builder; defaults false. Unit tests added.
- **Persistence** (gorm): new `preserve_context` column
  (`not null;default:false`), mapped both ways and in the partial-update
  map. AutoMigrate adds the column; existing rows backfill to false. Memory
  store stores the aggregate by value, so no mapper change.
- **Application** (`bots`, `lifecycle`): threaded `PreserveContext` through
  `CreateParams`/`UpdateParams` (update uses a `*bool` patch — nil = leave
  unchanged).
- **REST** (`BotDTO`, `CreateBotRequest`, `UpdateBotRequest`): exposed
  `preserve_context`; mapped in the create/update handlers and `botDTO`.
- **Runtime** (`spawner.go::ensureSession`): on re-activation, loads the bot
  and skips `ClearSession` when `PreserveContext` is true; logs the
  cleared-vs-preserved branch. Failing to read the bot propagates the error
  rather than silently wiping a preserve-context bot. New test
  `TestSpawnerPreservesContextWhenBotOptsIn`; existing clear-on-reactivation
  tests still pin the default-off behaviour.
- **Frontend** (`HelixOrgBotDetail.tsx`): a "Preserve context across
  triggers" switch wired through `useUpdateBot`, with help text covering the
  speed-vs-compaction trade-off. Regenerated OpenAPI/TS client.

## Notes

- MCP `create_bot`/`update_bot` intentionally do NOT expose this — keeping
  the MCP surface to org-graph primitives (helix-org philosophy); a bot
  shouldn't toggle its own context policy.
- Verified live in the inner Helix (helix-org enabled): the
  `preserve_context` field round-trips through `POST`/`GET`/`PATCH /bots`
  (create true→true, patch false→false with content untouched, patch
  true→true) and persists in the `org_bots.preserve_context` column. The
  spawner's skip-clear branch itself is proven by
  `TestSpawnerPreservesContextWhenBotOptsIn` (real Spawner + real store);
  capturing it in a live re-activation is timing-gated by per-worker
  activation serialization, so it was not observed in the live log.
