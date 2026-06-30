# Requirements: Fix Org Agent App Listing After Bot Table Rename

## Background

Listing apps for an org currently fails with:

```
failed to list helix-org agent apps: ERROR: relation "org_worker_runtime_state" does not exist (SQLSTATE 42P01)
```

This is a regression introduced by the **bot refactor** (spectask
`002185_i-need-to-merge-the` — "Merge Role and Worker into a single Bot").
That change merged the helix-org `Role` + `Worker` concepts into a single
`Bot`, and migration `0005_merge_roles_workers_into_bots.up.sql` **dropped**
`org_worker_runtime_state`. GORM AutoMigrate recreates the runtime-state
sidecar under the new name `org_bot_runtime_state`
(`api/pkg/org/infrastructure/persistence/gorm/bot_runtime.go:25`).

One straggler was missed: `markHelixOrgAgents` in
`api/pkg/server/app_handlers.go:395` still queries the old table name
`org_worker_runtime_state`. Because the table no longer exists, every org
app listing that reaches this code (i.e. when `HelixOrgEnabled` is true and
the org has apps) returns a 500.

## User Stories

**As an org owner with helix-org enabled**, when I open the apps list for my
org, I want it to load successfully so that I can see and manage my apps,
including which ones back org bots.

## Acceptance Criteria

- [ ] `markHelixOrgAgents` queries `org_bot_runtime_state` (the post-refactor
      table), not `org_worker_runtime_state`.
- [ ] Listing apps for an org with `HelixOrgEnabled=true` returns 200 (no
      SQLSTATE 42P01 error) whether or not the org has any bots.
- [ ] Apps that back a helix-org bot still have `IsHelixOrgAgent=true` set
      (the original behaviour is preserved — the runtime-state rows for the
      `helix` backend with key `agent_app_id` still identify them).
- [ ] No remaining references to `org_worker_runtime_state` in non-migration
      Go code.
- [ ] QA in the local Helix instance shows no errors in the API logs when
      listing org apps.

## Out of Scope

- Any change to the bot refactor schema or migrations.
- Re-keying runtime state from worker to bot (already handled by the
  refactor; rows are recreated by AutoMigrate).
