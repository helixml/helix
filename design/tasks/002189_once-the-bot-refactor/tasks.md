# Implementation Tasks: Per-Bot Toggle to Preserve Conversation Context Across Triggers

> The bot refactor (spectask `002185_i-need-to-merge-the`) is already
> merged into `main` (verified 2026-06-30). Build on the Bot symbols.

- [x] Domain: add `PreserveContext bool` to `orgchart.Bot` + `WithPreserveContext` builder; extend `bot_test.go`.
- [x] Gorm persistence: add `preserve_context` column to the bot row, map in `ToRow`/`ToDomain`, add to the `Update` `WithUpdates` map.
- [x] Memory store: no change needed — memory `botsRepo` stores the whole `orgchart.Bot` by value, so `PreserveContext` round-trips automatically.
- [x] Verify AutoMigrate adds the column and existing rows default to `false` — `botRow` is in `orgRowTypes` (gorm.go) and AutoMigrate runs over it; `gorm:"not null;default:false"` backfills existing rows.
- [x] Application: thread `PreserveContext` through the bot create + update use cases (`bots.CreateParams`/`UpdateParams` and `lifecycle.CreateParams`).
- [x] REST DTO: add `preserve_context bool` to `BotDTO` + `CreateBotRequest`, and `*bool` to `UpdateBotRequest` (patch-style, nil = unchanged); mapped in create/update handlers + `botDTO`.
- [skip] MCP: not added — REST/UI is the required surface and helix-org philosophy keeps the MCP surface to org-graph primitives. The agent should not toggle its own context policy.
- [x] Runtime: in `spawner.go::ensureSession`, load the bot and skip `ClearSession` when `PreserveContext` is true; log cleared-vs-preserved branch.
- [x] Spawner tests: `TestSpawnerPreservesContextWhenBotOptsIn` asserts no clear when on; existing `TestSpawnerClearsSessionOnReactivationOnly`/`FollowUpResumesPersistedSession` still pin clear-once when off.
- [x] Frontend: added a Switch toggle (`preserve_context`) to `HelixOrgBotDetail.tsx`, wired through `useUpdateBot` + the generated client; label + trade-off help text included. (Bot type is the generated `ApiBotDTO`, not a hand-written `types.ts`.)
- [x] `./stack update_openapi` — regenerated swagger + TS client (`preserve_context` now in `ApiBotDTO`/`CreateBotRequest`/`UpdateBotRequest`). `tsc --noEmit` passes clean; `vite build` compiles all modules (only fails copying into the root-owned `dist/` bind mount, an env issue, not a code issue).
- [x] Persistence/DTO round-trip: covered by the gorm + api test suites (all green) plus the new spawner test that creates/updates a preserve-context bot through the store.
- [ ] End-to-end in inner Helix: enable toggle on a bot, trigger twice via Slack auto-router, confirm the 2nd turn continues the same session/Zed thread; confirm a default-off bot still wipes (no regression).
- [~] `go build ./api/pkg/org/...`, run the org test suites, push and check CI.
