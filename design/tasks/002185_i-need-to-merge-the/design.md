# Design: Merge Role and Worker into a single Bot concept

## Where the code lives

`helix-org` is a self-contained DDD-layered subsystem under
`api/pkg/org/`:

- `domain/orgchart/` — the aggregate: `role.go`, `worker.go`, `kind.go`,
  `reporting.go`, `ids.go`, `validate.go`.
- `domain/{channels,streaming,activation,store,…}` — derived rules + ports.
- `application/{lifecycle,roles,workers,reconcile,dispatch,subscriptions,…}`
  — orchestration services.
- `infrastructure/persistence/{gorm,memory}` — `org_roles`, `org_workers`,
  `org_reporting_lines`, `org_worker_runtime_state`, etc.
- `interfaces/{mcptools,server/api}` — the MCP tools + REST DTOs/handlers.
- `frontend/src/pages/HelixOrg*.tsx`, `components/helix-org/`,
  `services/helixOrgService.ts`, `router.tsx`, `types.ts` — the UI.

## Core decision: Bot = old Role, structurally re-anchored

The new aggregate `orgchart.Bot` is the old `Role` plus the *structural*
role of the old `Worker`. We **keep** Role's fields and **drop** Worker's
identity fields:

| Old Role            | Old Worker                  | New Bot                      |
|---------------------|-----------------------------|------------------------------|
| `ID` (`r-…`)        | `ID` (`w-…`)                | `ID` (`b-…`)                 |
| `Content` (md)      | `IdentityContent` (md)      | `Content` (md) — *the only text* |
| `Tools`             | —                           | `Tools`                      |
| `Topics`            | —                           | `Topics`                     |
| `OrganizationID`    | `OrganizationID`            | `OrganizationID`             |
| timestamps          | `Kind` (human/ai)           | timestamps                   |
| —                   | `RoleID` (binding)          | *(gone — bot is the role)*   |

**Dropped entirely:** `WorkerKind` (`kind.go`, human/ai), `IdentityContent`,
the `RoleID` binding, the `Worker` interface + `HumanWorker`/`AIWorker`, and
the "many workers per role" / "live propagation" / "specialisation" semantics
(each bot is singular, so its tools/content edits are simply its own).

**Re-anchored onto the Bot** (these were Worker-anchored, now bot-anchored):
reporting lines, subscriptions, transcript/team/DM streams, runtime state
(`project_id`/`agent_app_id`/`session_id`), activations, project/agent
provisioning.

`Bot` is an immutable value with `With*` builders (mirrors current `Role`):

```go
type Bot struct {
    ID             BotID          // alias of string, convention "b-<name>"
    OrganizationID string
    Content        string
    Tools          []tool.Name
    Topics         []streaming.TopicID
    CreatedAt, UpdatedAt time.Time
}
func NewBot(id BotID, content string, tools []tool.Name, topics []streaming.TopicID, now time.Time, orgID string) (Bot, error)
```

`ReportingLine` keeps its shape but its `ManagerID`/`ReportID` are now
`BotID`. `channels.go` (`s-transcript-/s-team-/s-dm-`) and the
`reconcile` diff are unchanged in logic — they just take bot ids.

### Why not keep two types?
The user's intent is explicit: one concept, no identity beyond a name. A
1:1 Role↔Worker that we *call* one thing would keep the binding, the
propagation code, and the chart group-frames — i.e. the complexity we're
removing. Collapsing to one aggregate is what deletes code.

## Lifecycle simplification

`application/lifecycle` currently has **Hire** (create Worker + reporting
line + reconcile + hire-activation), **Fire** (teardown), and a separate
**DeleteRole** cascade. After the merge:

- **CreateBot** = one step: insert bot row, optional parent reporting line,
  reconcile topology, dispatch the create-activation. (Replaces "create_role
  then hire_worker".)
- **DeleteBot** = the old Fire + DeleteRole merged: tear down Helix
  project/app, drop subscriptions + reporting lines (FK cascade), reconcile,
  delete the row. Stays REST-only.

The `WorkerReconciler`/`OrgReconciler` seams, the dispatcher, and the helix
runtime spawner are unchanged except for the rename and the removal of
`Kind`/`IdentityContent` from their inputs (the spawner now projects the
bot's `Content` where it used to project `identity.md`; decide whether to
keep `identity.md` as the on-disk filename or rename to `bot.md`/`role.md` —
**recommend keeping `role.md`** since that is the prompt the agent reads).

## MCP tools (`interfaces/mcptools`)

| Remove / merge                         | New tool        |
|----------------------------------------|-----------------|
| `create_role` + `hire_worker`          | `create_bot`    |
| `update_role` + `update_identity`      | `update_bot`    |
| `read_roles` + `read_workers`          | `list_bots` / `get_bot` |

`managers`, `reports`, `dm`, `subscribe`, `unsubscribe`, `publish`,
`read_events`, `create_topic/stream`, `stream_members`, `ping`,
`mint_credential`, `*_worker_project` (→ `*_bot_project`) stay, retargeted
to bots. Baseline read tools (`managers`, `reports`, `read_events`, …) are
injected on `create_bot` exactly as `create_role` injects them today
(`defaults.go`). Update `registry.go` and `schema.go` accordingly. There is
no `delete_bot` MCP tool (parity with Fire being REST-only).

## REST API (`interfaces/server/api`)

- Routes: `/roles` + `/workers` → `/bots`. `/workers/{id}/parents`,
  `/activate`, `/chat`, `/exploratory-session`, etc. → `/bots/{id}/…`.
  `/workers/{id}/identity` is removed (content edit goes through the bot
  update).
- DTOs (`dto.go`): `RoleDTO` + `WorkerDTO` → **`BotDTO`**
  (`{id, content, tools, topics, parent_ids, created_at, updated_at}` — no
  `kind`, no `identity_content`, no `role_id`). `OrgOverview`/`RoleGroup`/
  `WorkerBadge` collapse to a flat bot list.
- The parity tests (`roles_workers_parity_test.go`, `streams_parity_test.go`)
  shrink to bot tests.

## Persistence (`infrastructure/persistence`)

- `org_roles` + `org_workers` → **`org_bots`** (`id, org_id` composite PK;
  columns `content`, `tools` json, `topics` json, timestamps).
- `org_reporting_lines`: `manager_id`/`report_id` now reference
  `org_bots(id, org_id)` ON DELETE CASCADE (mirror the existing FK
  installation in `gorm.go`'s post-AutoMigrate loop).
- `org_subscriptions`, `org_worker_runtime_state` →
  `org_bot_runtime_state`: `worker_id` column → `bot_id`.
- Drop the `worker`/`role` repos; add a single `Bots` repo + mapper.
  Update the `Store` struct, the `memory` store, and `gorm.go`'s
  `orgRowTypes` + FK loop + table-drop loop.

### Migration (follow `0004_drop_org_positions`) — keep it dead simple
Add `0005_merge_roles_workers_into_bots.up.sql`: guarded `DROP TABLE …
CASCADE` for the helix-org tables that change shape (`org_roles`,
`org_workers`, `org_reporting_lines`, `org_subscriptions`,
`org_worker_runtime_state`). AutoMigrate then creates the new tables
(`org_bots`, the re-keyed reporting/subscription/runtime tables) on the next
boot. **No re-bootstrap / owner-bot seeding logic** — the operator recreates
their bots manually after the schema change (helix-org is pre-release). A
header comment states this. Provide a `.down.sql` no-op matching the existing
convention.

## Frontend

- **Pages:** merge `HelixOrgRoles.tsx` + `HelixOrgWorkers.tsx` →
  `HelixOrgBots.tsx`; merge `HelixOrgRoleDetail.tsx` +
  `HelixOrgWorkerDetail.tsx` → `HelixOrgBotDetail.tsx` (content/tools editor
  + subscriptions + reporting + inline chat transcript + project/agent links
  + Advanced→Restart). Keep `HelixOrgChart`, `HelixOrgSettings`,
  `HelixOrgTopics*`.
- **Components:** `NewRoleDialog` + `HireWorkerDrawer` → `NewBotDialog`
  (id + content + optional parent). Drop the kind selector / identity field.
  `useHelixOrgBreadcrumbs` sections: Roles+Workers → Bots.
- **Chart:** remove Role group-frames; render bots as nodes; keep reporting
  / subscription drag edges retargeted to `/bots/{id}/parents` + subscribe.
- **Wiring:** `helixOrgService.ts` (role/worker calls → bot calls),
  `types.ts` (`Role`/`Worker` → `Bot`), `router.tsx` (`/roles`,
  `/roles/:id`, `/workers`, `/workers/:id` → `/bots`, `/bots/:id`),
  `HelixOrgSidebar.tsx`.

## QA

Rewrite `api/pkg/org/QA.md` to the Bot model: the "Mental model" section
drops Role/Worker/Kind/Identity and describes Bot + reporting line +
subscription; every `r-*`/`w-*` becomes `b-*`; §2 (role tool editor) and §3
(hire) merge into a "Bots list + create + tool editor" section; §10 (chat),
§12 (chart drag), §13 (managers/reports/dm), §16 (multi-tenancy) keep their
structure with bot ids. The rewritten plan is the script for the manual UI
verification in US-6.

## Risks

- **Breadth:** the rename touches ~60 Go files + ~15 frontend files. Mitigate
  by going layer-by-layer (domain → app → infra → interfaces → UI), compiling
  and running each layer's tests before moving outward.
- **Stream id continuity:** keep the `s-transcript-/s-team-/s-dm-` derivation
  identical (just bot ids) so the reconciler/topology tests change only in
  fixtures, not logic.
- **Hidden `worker`/`role` references** outside `api/pkg/org` (e.g. server
  wiring in `api/pkg/server/helix_org*.go`, sandbox MCP surface): grep the
  whole tree for the helix-org symbols, not just the package.
