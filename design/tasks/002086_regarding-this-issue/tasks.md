# Implementation Tasks: Tool Reconciler and Default Base Tools for Roles

- [x] Add `api/pkg/org/application/tools/defaults.go` exporting `BaseReadTools []tool.Name` containing: `managers`, `reports`, `list_workers`, `get_worker`, `list_roles`, `get_role`, `list_streams`, `get_stream`, `list_stream_events`, `read_events`, `worker_log`, `get_worker_environment`.
- [x] Add a startup-fail validation in `RegisterBuiltins` (`api/pkg/org/application/tools/builtins.go`) that every name in `BaseReadTools` resolves in the registry.
- [x] Refactor `api/pkg/org/application/bootstrap/bootstrap.go:77-101` so the owner defaults are `append(ownerMutationTools, tools.BaseReadTools...)` — no behavioural change to the resulting set.
- [x] Modify `api/pkg/org/application/tools/create_role.go:47-68` so `Invoke` merges `tools.BaseReadTools` with the caller-supplied `args.Tools` (union, order-stable, deduped) before passing to `orgchart.NewRole`.
- [x] Add `api/pkg/org/application/tools/reconciler.go` defining `RoleReconciler{Store, Now}` with `Reconcile(ctx, orgID) error`. Reconcile loads all roles for an org, computes the union with `BaseReadTools`, and calls `Roles.Update` only when the set changes.
- [x] Wire `RoleReconciler.Reconcile` into `helix_org_middleware.ensureBootstrap` right after `topology.Reconciler.ReconcileAll`. Log errors but do not break the request (matches the topology reconcile's best-effort pattern). Runs once per org per process via the existing `bootstrapped` flag.
- [x] Add `defaults_test.go`: assert `BaseReadTools` matches a golden list; assert every name resolves in the registry.
- [x] Add `create_role_test.go` cases for `tools:[]` and for `tools:[publish, managers]` (verifies union, dedup, order preservation).
- [x] Add `reconciler_test.go`: seed a role with `tools:[dm, publish]`, run `Reconcile`, assert the role's tools is the union with `BaseReadTools`; run `Reconcile` again, assert no `Roles.Update` write occurred and `UpdatedAt` is unchanged.
- [x] Extend the E2E test in `api/pkg/org/application/tools/builtins_test.go` (`TestDemoOwnerHiresCEO` and/or a sibling) to assert that an MCP `tools/list` on `w-owner` includes both `managers` and `reports`, and that a CEO-like role created via `create_role` with a minimal tool list still exposes the full base read set on its MCP surface.
- [x] Run local build + the affected test packages, merge origin/main into the feature branch, push.
- [x] Write `pull_request_helix.md` describing the change and explicitly noting Failure 2 (`dm` tool ignoring the reporting graph) is out of scope.
- [~] Watch CI on the pushed feature branch; if anything fails, drill into Drone logs and fix.
