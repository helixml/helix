# fix(api): query org_bot_runtime_state after bot table rename

## Summary

Listing apps for an org failed with
`failed to list helix-org agent apps: ERROR: relation "org_worker_runtime_state" does not exist (SQLSTATE 42P01)`.

This was a straggler from the bot refactor (spectask `002185`), which merged the
helix-org `Role` + `Worker` concepts into a single `Bot` and dropped the
`org_worker_runtime_state` table (migration `0005`). GORM AutoMigrate recreates
the runtime-state sidecar under the new name `org_bot_runtime_state`, but
`markHelixOrgAgents` in `app_handlers.go` still queried the old, now-dropped
table — so every org app listing (with helix-org enabled) returned a 500.

## Changes

- `api/pkg/server/app_handlers.go`: `markHelixOrgAgents` now queries
  `org_bot_runtime_state` instead of `org_worker_runtime_state`; the accompanying
  comment is updated to the new schema (org, bot, backend, key). The query's
  filter columns are unchanged across the rename, so the table name was the only
  functional change.
- `api/pkg/server/app_handlers_test.go`: fixed a stale doc-comment referencing the
  old table/"Worker" wording (no test logic changed).

## Testing

- `go build ./...` (in `api/`) passes.
- End-to-end in the local inner Helix with `HELIX_ORG_ENABLED=true`: registered an
  admin user, created an org and an app in it, then
  `GET /api/v1/apps?organization_id=<org>` returned **200** (exercising the
  `org_bot_runtime_state` query). API logs showed **0** occurrences of `42P01`,
  `org_worker_runtime_state`, or `failed to list helix-org agent apps`.
