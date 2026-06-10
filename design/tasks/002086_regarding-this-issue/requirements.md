# Requirements: Tool Reconciler and Default Base Tools for Roles

## Background

Source: [helixml/helix#2546](https://github.com/helixml/helix/issues/2546).

The §13 QA regression found that `managers` and `reports` MCP tools were missing across all roles — including the freshly-bootstrapped `r-owner` and the manually-created `r-qa-engineer`. The root cause is that the tool surface for any role is **only** what its creator chose to write into `Role.Tools` (see `api/pkg/org/application/tools/create_role.go:60` and `api/pkg/org/application/bootstrap/bootstrap.go:77-101`). There is no enforced baseline — when a role's `tools` list omits a basic read tool, every Worker holding that role permanently loses access to it, because the MCP server is built live from `Role.Tools` on every request (`api/pkg/org/interfaces/server/mcp.go:68-120`).

This affects every role created today: any caller of `create_role` that forgets a base tool (e.g. `managers`, `reports`, the `read_*` family) ships workers that cannot self-introspect their reporting graph, observe streams, or look up other workers — all primitives §13 (and several other QA sections) rely on.

## User Stories

**US-1.** As a Helix-Org operator, I want every Role — existing and future — to expose a guaranteed base set of read tools (`managers`, `reports`, `list_workers`, `get_worker`, `list_roles`, `get_role`, `list_streams`, `get_stream`, `list_stream_events`, `read_events`, `worker_log`, `get_worker_environment`), so that no Worker is ever unable to introspect its reporting graph or read streams it has been subscribed to.

**US-2.** As an Owner Worker using the `create_role` tool, I want the base read tools to be injected automatically when I create a new Role, so that I only need to specify the *additional* tools that role needs (mutations, scoped writes) without re-listing the same boilerplate every time.

**US-3.** As an Owner Worker who already has Roles from before this change, I want existing Roles to be backfilled with the base read tools on the next reconcile, so that I don't have to manually call `update_role` on every Role to fix the regression.

**US-4.** As a Helix-Org developer, I want a single, named source of truth for the "base read tool set" so that changes to the baseline (adding/removing a default) happen in one place and apply uniformly to bootstrap, `create_role`, and the reconciler.

## Acceptance Criteria

**AC-1 — Single source of truth.** A new exported symbol (e.g. `tools.BaseReadTools`) lists the base read tool names. Bootstrap, `create_role`, and the reconciler all reference this single list — no duplication.

**AC-2 — Bootstrap unchanged in behaviour.** `r-owner` continues to be created with its existing union of mutation + read tools. The defaults slice in `bootstrap.go` is refactored to compose `tools.BaseReadTools` with the owner-only mutation tools; the resulting set is identical to today's (the bug-fix happens via the reconciler for already-bootstrapped orgs, not by changing bootstrap behaviour).

**AC-3 — `create_role` injects defaults.** When `create_role` is invoked, the resulting `Role.Tools` is the union of `tools.BaseReadTools` and the caller-supplied `tools` array, de-duplicated and order-stable. A caller that supplies an empty/missing `tools` field still gets the base set. A caller that supplies tools already in the base set does not get duplicates.

**AC-4 — Tool Reconciler exists.** A new `tools.RoleReconciler` (or equivalent package) provides `Reconcile(ctx, orgID) error` which, for every Role in the org, ensures `Role.Tools` is a superset of `tools.BaseReadTools`. Missing names are appended; existing extras (mutations, custom tools) are preserved; order is stable; the call is idempotent.

**AC-5 — Reconciler runs at the right times.** The reconciler is invoked:
  - Once at the end of bootstrap (after `r-owner` is created — a no-op there, but it proves the wiring).
  - On every API-server start, scoped to all orgs (one-shot, not a background loop — keeps it simple and predictable).

**AC-6 — Worker tool surface follows the Role.** No change to the MCP server build path — `Role.Tools` remains the live SSoT. Because the reconciler edits `Role.Tools` in-place, every Worker holding the updated Role sees the new tools on its next MCP request. No per-Worker state to migrate.

**AC-7 — Regression test for §13.** A test fixture creates an org, runs bootstrap, then asserts that `tools/list` for `w-owner` includes both `managers` and `reports`. A second fixture creates a Role via `create_role` with an empty `tools:[]` and asserts the resulting MCP surface still includes the full base read set.

**AC-8 — Backfill test.** A test seeds a Role with `tools: [dm, publish]` (simulating a pre-fix `r-qa-engineer`), runs the reconciler, and asserts the Role's tool list is now the union with `tools.BaseReadTools` — and that a second reconcile is a no-op (no writes, no churn).

## Out of Scope

- Failure 2 in issue #2546 (the `dm` tool not enforcing the reporting graph). That is a separate logic bug in `dm.go` and should be its own spec/PR.
- Per-Worker tool overrides (none exist today; introducing them is a much bigger design conversation).
- A background reconcile loop. One-shot at startup + at bootstrap is enough; we can add periodic reconcile later if drift becomes a real concern.
- Removing tools from existing roles. The reconciler only **adds** missing base tools; it never subtracts. Owners can still call `update_role` to prune.
