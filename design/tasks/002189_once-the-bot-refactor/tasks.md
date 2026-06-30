# Implementation Tasks: Per-Bot Toggle to Preserve Conversation Context Across Triggers

> The bot refactor (spectask `002185_i-need-to-merge-the`) is already
> merged into `main` (verified 2026-06-30). Build on the Bot symbols.

- [~] Domain: add `PreserveContext bool` to `orgchart.Bot` + `WithPreserveContext` builder; extend `bot_test.go`.
- [ ] Gorm persistence: add `preserve_context` column to the bot row, map in `ToRow`/`ToDomain`, add to the `Update` `WithUpdates` map.
- [ ] Memory store: carry `PreserveContext` in the in-memory bot mapper/copy.
- [ ] Verify AutoMigrate adds the column and existing rows default to `false` (no SQL migration needed).
- [ ] Application: thread `PreserveContext` through the bot update use case via `WithPreserveContext`.
- [ ] REST DTO: add `preserve_context bool` to `BotDTO` + `CreateBotRequest`, and `*bool` to `UpdateBotRequest` (patch-style, nil = unchanged); map in handlers.
- [ ] (Optional) MCP: add optional `preserve_context` arg to `update_bot` in `schema.go` + handler — only if needed; keep MCP surface small.
- [ ] Runtime: in `spawner.go::ensureSession`, load the bot and skip `ClearSession` when `PreserveContext` is true; log cleared-vs-preserved branch.
- [ ] Spawner tests: assert `ClearSession` is skipped when `PreserveContext=true` and still runs once (before `SendMessage`) when false.
- [ ] Frontend: add the toggle to `HelixOrgBotDetail.tsx`, wire through `helixOrgService.ts` + `Bot` type in `types.ts` (generated API client); add label + trade-off help text.
- [ ] `./stack update_openapi` if the DTO changed; `cd frontend && yarn build`.
- [ ] Persistence/DTO round-trip unit tests for the new field.
- [ ] End-to-end in inner Helix: enable toggle on a bot, trigger twice via Slack auto-router, confirm the 2nd turn continues the same session/Zed thread; confirm a default-off bot still wipes (no regression).
- [ ] `go build ./api/pkg/org/...`, run the org test suites, push and check CI.
