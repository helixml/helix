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
