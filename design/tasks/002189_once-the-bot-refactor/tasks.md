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
- [~] Runtime: in `spawner.go::ensureSession`, load the bot and skip `ClearSession` when `PreserveContext` is true; log cleared-vs-preserved branch.
- [ ] Spawner tests: assert `ClearSession` is skipped when `PreserveContext=true` and still runs once (before `SendMessage`) when false.
- [ ] Frontend: add the toggle to `HelixOrgBotDetail.tsx`, wire through `helixOrgService.ts` + `Bot` type in `types.ts` (generated API client); add label + trade-off help text.
- [ ] `./stack update_openapi` if the DTO changed; `cd frontend && yarn build`.
- [ ] Persistence/DTO round-trip unit tests for the new field.
- [ ] End-to-end in inner Helix: enable toggle on a bot, trigger twice via Slack auto-router, confirm the 2nd turn continues the same session/Zed thread; confirm a default-off bot still wipes (no regression).
- [ ] `go build ./api/pkg/org/...`, run the org test suites, push and check CI.
