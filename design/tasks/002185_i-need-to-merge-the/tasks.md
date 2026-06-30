# Implementation Tasks: Merge Role and Worker into a single Bot concept

Work layer-by-layer (DDD), TDD where possible: update each layer's
`_test.go` to the new shape first, then make it compile and pass before
rippling outward. Build/test after each layer.

## 1. Domain (`api/pkg/org/domain`) [~]
- [x] Add `orgchart/bot.go`: `Bot` value + `NewBot` + `With*` builders (fields: ID, OrganizationID, Content, Tools, Topics, timestamps); add `BotID` alias to `ids.go`.
- [x] Delete `orgchart/worker.go`, `orgchart/kind.go` (and their tests); remove the `Worker` interface, `HumanWorker`/`AIWorker`, `WorkerKind`.
- [x] Delete `orgchart/role.go` (folded into `bot.go`); update `validate.go`/`ids.go` package docs and any `RoleID`/`WorkerID` references.
- [x] Repoint `reporting.go` (`ManagerID`/`ReportID` → `BotID`); keep DAG/self-report rules.
- [x] Update `domain/channels`, `domain/activation`, `domain/streaming` (subscription principal) to bot ids; keep `s-transcript-/s-team-/s-dm-` derivation identical.
- [x] Update `domain/store/store.go`: replace `Roles` + `Workers` ports with a single `Bots` port; rename `WorkerRuntimeState` → `BotRuntimeState`; retarget `ReportingLines`/`Subscriptions`/`Events` signatures to `BotID`.
- [x] domain tests green

## 2. Application (`api/pkg/org/application`)
- [x] Merge `application/roles` + `application/workers` into one `application/bots` service (create/get/list/update/delete).
- [x] Collapse `application/lifecycle`: `Hire`+`DeleteRole` → `CreateBot`; `Fire` → `DeleteBot` (merge teardown); keep reconciler/dispatcher/runtime seams.
- [x] Update `application/reconcile`, `dispatch`, `subscriptions`, `transcript`, `prompts` (role.go/templates/role.md), `slackrouting` to bot ids/types.
- [x] application tests green

## 3. Infrastructure (`api/pkg/org/infrastructure`)
- [x] Replace `gorm/role.go` + `gorm/worker.go` with `gorm/bot.go` (`org_bots`, composite PK, json tools/topics); update `gorm/reporting_line.go`, `gorm/subscription.go`, `worker_runtime.go`→`bot_runtime.go` to `bot_id`.
- [x] Update `gorm/gorm.go`: `orgRowTypes`, FK-install loop (reporting lines → `org_bots` ON DELETE CASCADE), and removed-table drop loop.
- [x] Update the `memory` store to the `Bots` port + bot-anchored maps.
- [x] Update `infrastructure/runtime/helix` (spawner, identity→content projection, hire, project, mcp, mirror, state) to bots; keep `role.md` on-disk filename.
- [x] infra tests compile; memory tests green (gorm/runtime need Postgres)

## 4. Interfaces — MCP tools (`api/pkg/org/interfaces/mcptools`)
- [x] `create_role`+`hire_worker` → `create_bot`; `update_role`+`update_identity` → `update_bot`; `read_roles`+`read_workers` → `list_bots`/`get_bot`.
- [x] Retarget `managers`, `reports`, `dm`, `subscribe`, `unsubscribe`, `publish`, `read_events`, `stream_members`, `*_worker_project`→`*_bot_project`, `invite_workers`, `worker_log`→`bot_log` to bots.
- [x] Update `registry.go`, `schema.go`, `defaults.go` (baseline tool injection on `create_bot`); update tool `_test.go`.

## 5. Interfaces — REST (`api/pkg/org/interfaces/server/api` + `server`)
- [x] `dto.go`: `RoleDTO`+`WorkerDTO` → `BotDTO`; collapse `OrgOverview`/`RoleGroup`/`WorkerBadge`.
- [x] Merge `roles.go`+`workers.go` → `bots.go`; route `/roles`+`/workers` → `/bots`, move `/parents`, `/activate`, `/chat`, `/exploratory-session` to `/bots/{id}/…`; drop `/identity`.
- [x] Update `server.go`/`mcp.go`/`overview.go`/`subscriptions.go` wiring and the parity tests (`roles_workers_parity_test.go` etc.); update `api/pkg/server/helix_org*.go` composition root.
- [x] Full repo builds; `go test ./api/pkg/org/...` green; swagger + TS client regenerated.

## 6. Database migration
- [x] Add `api/pkg/store/migrations/0005_merge_roles_workers_into_bots.{up,down}.sql` — keep it simple: guarded `DROP TABLE` for the changed helix-org tables (`org_roles`, `org_workers`, `org_reporting_lines`, `org_subscriptions`, `org_worker_runtime_state`), AutoMigrate creates the new ones. No re-bootstrap/seeding (operator recreates bots manually); `.down` no-op.

## 7. Frontend (`frontend/src`)
- [x] Merge `HelixOrgRoles`+`HelixOrgWorkers` → `HelixOrgBots.tsx`; merge `HelixOrgRoleDetail`+`HelixOrgWorkerDetail` → `HelixOrgBotDetail.tsx`.
- [x] `NewRoleDialog`+`HireWorkerDrawer` → `NewBotDialog`; drop kind selector + identity field.
- [x] Update `HelixOrgChart.tsx` (bot nodes, no role frames), `HelixOrgSidebar.tsx`, `useHelixOrgBreadcrumbs.ts` (Bots section).
- [x] Update `helixOrgService.ts`, `types.ts`, `router.tsx` (`/roles`,`/workers` routes → `/bots`).
- [x] `yarn build` green; obsolete `HelixOrgWorkerDetail.test.tsx` removed.

## 8. QA rewrite + global sweep
- [x] Rewrite `api/pkg/org/QA.md` to the Bot model (mental model, `b-*` ids, merged sections).
- [x] Swept tree; production builds clean. Fixed agent prompt ("You are Bot"). Internal runtime path/env (`HELIX_WORKER_ID`, `workers/<id>/`) intentionally kept (plumbing).

## 9. End-to-end verification (local Helix UI)
- [x] Built/ran in-sandbox Helix (HELIX_ORG_ENABLED=true + alpha flag); registered test@helix.ml.
- [x] Verified core flows via UI: empty-org seed, create bot (chart+dialog), Bots list, bot detail (chat/tools/subscriptions/project+agent links), reporting reconcile (transcript/team/DM topics + subscriptions confirmed in DB). Cycle-guard/reparent-unsubscribe and multi-tenancy §16 are covered by the passing Go unit/integration tests rather than hand-walked in the UI.
- [x] Verified delete-bot cascade (project teardown + reporting/subscription/topic cleanup) via UI; no defects surfaced. Feature branch pushed.
