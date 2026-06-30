# Design: Fix Org Agent App Listing After Bot Table Rename

## Root Cause

The bot refactor renamed the helix-org runtime-state table:

- Old: `org_worker_runtime_state` (dropped by migration
  `0005_merge_roles_workers_into_bots.up.sql`).
- New: `org_bot_runtime_state` — defined by `botRuntimeStateRow.TableName()`
  in `api/pkg/org/infrastructure/persistence/gorm/bot_runtime.go:25`,
  created on boot by GORM AutoMigrate.

`markHelixOrgAgents` (`api/pkg/server/app_handlers.go:377-416`) reaches into
helix-org's shared Postgres connection via the `GormDB()` accessor and runs a
raw query against the runtime-state table to find which apps back org bots
(the `helix` backend's `agent_app_id` value). It still names the old table,
so the query fails with `relation ... does not exist`.

## Decision

Minimal, surgical fix: update the table name (and the adjacent comment) in
`markHelixOrgAgents`. The query's filter columns are unchanged across the
refactor:

| Column    | Old table                  | New table              |
|-----------|----------------------------|------------------------|
| `org_id`  | yes                        | yes                    |
| `backend` | yes                        | yes                    |
| `key`     | yes                        | yes                    |
| `value`   | yes                        | yes                    |

The query filters on `org_id`, `backend = 'helix'`, `key = 'agent_app_id'`,
`value <> ''` and plucks distinct `value`. None of these columns were renamed
(the only schema change relevant here is `worker_id` → `bot_id`, which this
query does not reference). So the table name is the only change required.

### Change

In `api/pkg/server/app_handlers.go`:

```go
// org_bot_runtime_state holds one row per (org, bot, backend, key).
// The "helix" backend's "agent_app_id" value is the App backing that
// Bot — see api/pkg/org/infrastructure/runtime/helix/state.go.
...
    Table("org_bot_runtime_state").
    Where("org_id = ? AND backend = ? AND key = ? AND value <> ?", orgID, "helix", "agent_app_id", "").
    Distinct("value").
    Pluck("value", &agentAppIDs)
```

## Alternatives Considered

- **Route through the org store's `BotRuntimeState` repository instead of a
  raw `GormDB()` query.** Cleaner in principle, but `markHelixOrgAgents`
  needs to scan *all* bots in an org by `(backend, key)`, whereas the repo's
  `Get` is keyed by a specific `botID`. Adding a new repo method is more
  surface area than this regression warrants. Keep the existing raw-query
  approach; just fix the name. (Worth a follow-up if the raw query proves
  fragile again.)

## Testing

1. Build: `go build ./api/pkg/server/` (or `cd api && CGO_ENABLED=0 go build ./...`).
2. End-to-end QA in the local inner Helix (`http://localhost:8080`): register
   / sign in, ensure helix-org is enabled, list apps for the org, and confirm
   the response is 200 and the API logs contain no `42P01` /
   `failed to list helix-org agent apps` error.
3. Confirm no other non-migration Go references to the old table name remain:
   `grep -rn org_worker_runtime_state api/ --include=*.go`.

## Implementation Notes

- **Files changed (helix repo):**
  - `api/pkg/server/app_handlers.go` — `markHelixOrgAgents` now queries
    `org_bot_runtime_state` (was `org_worker_runtime_state`); comment updated.
  - `api/pkg/server/app_handlers_test.go` — fixed a stale doc-comment that still
    referenced the old table/"Worker" wording (no test logic changed).
- The query's filter columns (`org_id`, `backend`, `key`, `value`) are identical
  in the new table, so the table name was the only functional change.
- `go build ./...` in `api/` passes.

## QA Results (verified end-to-end, local inner Helix at localhost:8080)

The default dev config has `HELIX_ORG_ENABLED=false`, under which
`markHelixOrgAgents` short-circuits and the org runtime-state table is never
AutoMigrated (so the bug can't even reproduce). To exercise the real code path:

1. Set `HELIX_ORG_ENABLED=true` in `.env` and recreated the `api` container.
   On boot, GORM AutoMigrate created `org_bot_runtime_state` (confirmed via
   `SELECT to_regclass('org_bot_runtime_state')`).
2. Registered admin user `test@helix.ml`, created org `testorg`
   (`org_01kwcpx8…`), and created one app in that org via the API.
3. `GET /api/v1/apps?organization_id=org_01kwcpx8…` → **HTTP 200**, returned the
   app. Because the org is enabled, an orgID is supplied, and the org has ≥1 app,
   `markHelixOrgAgents` ran its `org_bot_runtime_state` query. A query failure
   would have surfaced as HTTPError500; it returned 200.
4. API logs over the test window: **0** matches for `42P01`,
   `org_worker_runtime_state`, or `failed to list helix-org agent apps`.

Note: `.env` is gitignored; the `HELIX_ORG_ENABLED=true` flip is local to the QA
instance and is not part of the committed change.
