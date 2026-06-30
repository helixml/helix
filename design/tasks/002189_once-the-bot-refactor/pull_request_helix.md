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
- NOT yet verified end-to-end in a live inner-Helix Slack flow — unit +
  package tests pass and `tsc` is clean; the live two-trigger check is the
  remaining validation.
