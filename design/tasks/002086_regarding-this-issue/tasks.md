# Implementation Tasks: Tool Reconciler and Default Base Tools for Roles

- [x] Add `api/pkg/org/application/tools/defaults.go` exporting `BaseReadTools []tool.Name` containing: `managers`, `reports`, `list_workers`, `get_worker`, `list_roles`, `get_role`, `list_streams`, `get_stream`, `list_stream_events`, `read_events`, `worker_log`, `get_worker_environment`.
- [x] Add a startup-fail validation in `RegisterBuiltins` (`api/pkg/org/application/tools/builtins.go`) that every name in `BaseReadTools` resolves in the registry.
- [x] Refactor `api/pkg/org/application/bootstrap/bootstrap.go:77-101` so the owner defaults are `append(ownerMutationTools, tools.BaseReadTools...)` — no behavioural change to the resulting set.
- [~] Modify `api/pkg/org/application/tools/create_role.go:47-68` so `Invoke` merges `tools.BaseReadTools` with the caller-supplied `args.Tools` (union, order-stable, deduped) before passing to `orgchart.NewRole`.
- [ ] Add `api/pkg/org/application/tools/reconciler.go` defining `RoleReconciler{Store, Now}` with `Reconcile(ctx, orgID) error` and `ReconcileAll(ctx) error`. Reconcile loads all roles for an org, computes the union with `BaseReadTools`, and calls `Roles.Update` only when the set changes.
- [ ] At the end of `bootstrap.Run` (after `topology.Reconcile`), call `RoleReconciler.Reconcile(ctx, params.OrganizationID)` as a wiring smoke-test.
- [ ] In `api/cmd/helix/serve.go`, after the org store is open and before the HTTP server starts accepting traffic, call `RoleReconciler.ReconcileAll(ctx)`. Log per-org errors but do not abort startup.
- [ ] Add `defaults_test.go`: assert `BaseReadTools` matches a golden list; assert every name resolves in the registry.
- [ ] Add `create_role_test.go` cases for `tools:[]` and for `tools:[publish, managers]` (verifies union, dedup, order preservation).
- [ ] Add `reconciler_test.go`: seed a role with `tools:[dm, publish]`, run `Reconcile`, assert the role's tools is the union with `BaseReadTools`; run `Reconcile` again, assert no `Roles.Update` write occurred and `UpdatedAt` is unchanged.
- [ ] Extend the E2E test in `api/pkg/org/application/tools/builtins_test.go` (`TestDemoOwnerHiresCEO` and/or a sibling) to assert that an MCP `tools/list` on `w-owner` includes both `managers` and `reports`, and that a CEO-like role created via `create_role` with a minimal tool list still exposes the full base read set on its MCP surface.
- [ ] Run `go build ./api/pkg/...` and the relevant unit-test subset locally, then push and confirm CI green via `gh pr checks` / Drone MCP tools.
- [ ] Open the PR against `helixml/helix` with full URL referenced back to issue [#2546](https://github.com/helixml/helix/issues/2546). The PR description should explicitly note Failure 2 (`dm` tool ignoring the reporting graph) is **out of scope** for this PR.
