# Add RoleReconciler and inject baseline reads into create_role

Fixes Failure 1 from https://github.com/helixml/helix/issues/2546 —
every helix-org Role now exposes the universal read baseline
(`managers`, `reports`, the `list_*` / `get_*` family, `read_events`,
`worker_log`, `get_worker_environment`), so no Worker can be created
without the primitives §13 (and many other QA sections) rely on.

## Summary

A single source of truth — `tools.BaseReadTools` — defines the reads
every Role must expose. Both code paths that produce Roles compose
from this list:

- **`create_role`** unions `BaseReadTools` into the caller-supplied
  tools (order-stable, deduped) before constructing the Role.
- **`RoleReconciler`** loads every Role in an org and appends any
  missing baseline names. Wired into `helix_org_middleware.ensureBootstrap`
  next to the existing topology reconciler, so it runs once per org per
  process and retroactively heals Roles created before the baseline
  existed.

`bootstrap.go`'s owner defaults are refactored to compose from
`BaseReadTools` too — keeping the read list defined once.

## Out of scope

**Failure 2** in the same issue (the `dm` tool not enforcing the
reporting graph) is a separate logic bug in `dm.go` and is intentionally
left for a follow-up PR. The two failures don't share code paths and
bundling them would obscure the review.

## Changes

- `api/pkg/org/application/tools/defaults.go` *(new)* — `BaseReadTools`
  list + `mergeBaseReadTools` helper.
- `api/pkg/org/application/tools/reconciler.go` *(new)* — `RoleReconciler`
  with nil-safe `Reconcile(ctx, orgID)`.
- `api/pkg/org/application/tools/create_role.go` — merge baseline into
  caller-supplied tools.
- `api/pkg/org/application/tools/builtins.go` — fail-fast at startup if
  `BaseReadTools` references an unregistered name.
- `api/pkg/org/application/bootstrap/bootstrap.go` — owner defaults now
  compose `ownerMutationTools` + `tools.BaseReadTools`.
- `api/pkg/server/helix_org_middleware.go` — wire `RoleReconciler.Reconcile`
  into `ensureBootstrap` right after `topology.Reconciler.ReconcileAll`,
  same best-effort error handling.

## Tests

- `defaults_test.go` — golden list, registry resolution, merge dedup +
  order + idempotency.
- `create_role_test.go` — empty caller tools get the full baseline;
  mixed list preserves caller order, dedups, appends baseline tail.
- `reconciler_test.go` — backfill on a §13-style drifted Role;
  idempotent (no rewrite on second pass, `UpdatedAt` unchanged); scoped
  to the requested org; nil-safe.
- `builtins_test.go` — two new E2E tests over the MCP endpoint:
  - `TestBootstrapOwnerHasBaselineReadsOverMCP` drives `bootstrap.Run`
    and asserts every `BaseReadTools` entry is on the `w-owner`
    `tools/list` (the original §13 observation).
  - `TestCreateRoleInjectsBaselineOverMCP` creates a QA-engineer-style
    role with only mutation tools listed; the hired worker still sees
    every baseline tool plus the caller-supplied ones.

## Test plan

- [ ] `CGO_ENABLED=1 go test ./api/pkg/org/application/tools/ ./api/pkg/org/application/bootstrap/ ./api/pkg/org/interfaces/server/ -count=1` is green locally.
- [ ] Drone build for the feature branch is green.
- [ ] In a deployed environment: create a fresh org, hit `/api/v1/mcp/helix-org/<org>/workers/w-owner/mcp` `tools/list`, confirm `managers` and `reports` are present.
- [ ] In a deployed environment: against an org that previously had `r-qa-engineer` missing reads, restart the API and confirm a single request to any `helix-org` endpoint triggers `RoleReconciler` (visible in the API logs) and that the QA-engineer role's `tools/list` now includes the baseline.
