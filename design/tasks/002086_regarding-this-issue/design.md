# Design: Tool Reconciler and Default Base Tools for Roles

## Context

The helix-org runtime already treats `Role.Tools` as the **single source of truth** for what MCP tools a Worker sees — the MCP handler rebuilds the per-Worker server live from the Role on every request (`api/pkg/org/interfaces/server/mcp.go:68-120`). There is no per-Worker tool state and no drift risk *between* a Role and its Workers. The actual drift problem is at a higher altitude: between **what every Role should expose by default** and **what an individual `create_role` caller remembered to write**.

This design therefore avoids reconciling Workers (there is nothing to reconcile — they derive from Roles). It reconciles **Roles against a declared default baseline**.

This also keeps us inside the helix-org design philosophy spelled out in `helix/CLAUDE.md` ("No workflow in code") — we are not orchestrating multi-step agent behaviour; we are enforcing a structural invariant on a domain object (`Role.Tools ⊇ BaseReadTools`).

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│ api/pkg/org/application/tools/                               │
│                                                              │
│   defaults.go        ← NEW. Exports BaseReadTools []tool.Name│
│                                                              │
│   create_role.go     ← MODIFIED. Merges BaseReadTools into   │
│                        the caller-supplied tools slice.      │
│                                                              │
│   reconciler.go      ← NEW. RoleReconciler.Reconcile(ctx,    │
│                        orgID) — appends any missing names    │
│                        from BaseReadTools to every Role,     │
│                        idempotent.                           │
└──────────────────────────────────────────────────────────────┘
            ▲                              ▲
            │ used by                      │ used by
            │                              │
┌───────────┴───────────────┐  ┌───────────┴───────────────────┐
│ bootstrap.go              │  │ cmd/helix/serve.go            │
│ (composes defaults +      │  │ (one-shot reconcile-all-orgs  │
│  owner mutations; ends    │  │  at API server start, after   │
│  with a Reconcile call    │  │  the store is open)           │
│  as a smoke-test no-op)   │  │                               │
└───────────────────────────┘  └───────────────────────────────┘
```

No new package is needed — everything lives under `api/pkg/org/application/tools/`, alongside the existing `Registry` and tool implementations.

## Key Decisions

### D1 — The "base set" is read-only, not "read + safe-mutation"

Picking the right baseline is the highest-leverage decision. The principle: a tool belongs in `BaseReadTools` only if exposing it to **every** role is safe and useful. We restrict to **reads** (and self-introspection) because:

- Reads cannot harm the org graph; mutations can. A misconfigured `r-qa-engineer` with `hire_worker` injected by default would be a security/scope regression worse than the bug we are fixing.
- The user request explicitly named the missing tools as "Managers and Reports tools and the Read Stream and other basic read functionality" — reads.
- Mutation tools belong in the owner role and in any role whose prompt explicitly delegates that authority. They stay opt-in via the `create_role` caller's `tools:[]` argument.

**Concrete baseline (matches the read-only subset of the existing bootstrap defaults):**

```
managers, reports,
list_workers, get_worker,
list_roles, get_role,
list_streams, get_stream, list_stream_events,
read_events, worker_log,
get_worker_environment
```

`get_worker_project` is intentionally **excluded** for now — it requires a configured project on the worker and would error noisily for workers without one. Re-evaluate after the project tooling stabilises.

### D2 — Union semantics, never subtract

The reconciler computes `Role.Tools = union(Role.Tools, BaseReadTools)` preserving the original order and appending any missing baseline names at the end. This is:

- **Idempotent** — a second run is a no-op (no write, no `UpdatedAt` bump).
- **Conservative** — never removes a caller's custom tool. Owners stay in control of pruning via `update_role`.
- **Order-stable** — the caller-supplied order is preserved (matters for any UI that lists tools in declaration order).

### D3 — One-shot per-org in `ensureBootstrap` (revised after discovery)

**Discovery during implementation:** The org graph is per-org and **lazy**. There is no "API server start" hook that iterates all orgs — `bootstrap.Run` is invoked from `helix_org_middleware.ensureBootstrap` on the first request for an org, and the existing `topology.Reconciler.ReconcileAll(ctx, orgID)` runs there too (`api/pkg/server/helix_org_middleware.go:134-137`).

So the right place for the new reconciler is exactly the same site, right after the topology reconcile. This:

- Runs once per org per process (the `bootstrapped` flag guards re-entry).
- Covers both first-time bootstrap (a no-op — `r-owner` already includes the baseline because `bootstrap.go` composes from `BaseReadTools`) **and** the upgrade case (catches pre-existing roles missing the baseline — the actual bug from issue #2546).
- Avoids needing access to a global org list (which doesn't exist in the org-store).
- Matches the project's existing reconciler convention so future maintainers find it where they expect.

We deliberately avoid a background loop. Drift can only happen on two events: (a) the baseline definition changes in code, or (b) `update_role` is misused. (a) is solved on next request after a deploy; (b) is a user-driven action and shouldn't be silently undone.

### D4 — Reconciler is a function, not a goroutine

```go
type RoleReconciler struct {
    Store *store.Store
    Now   func() time.Time
}

func (r *RoleReconciler) Reconcile(ctx context.Context, orgID string) error
```

Mirrors the shape of `topology.Reconciler` (`api/pkg/org/application/topology/reconciler.go:16-32`), which is the project's existing reconciler pattern. Same `Now` seam for tests.

No `ReconcileAll` is needed (see D3 — there is no global org list in the org-store; reconcile is per-org-on-first-request, same as topology).

### D5 — Compose, don't duplicate, the bootstrap defaults

Today `bootstrap.go:77-101` hard-codes a list of 23 tool names that overlaps almost entirely with the proposed `BaseReadTools`. Refactor it to:

```go
ownerMutationTools := []tool.Name{
    tools.CreateRoleName, tools.UpdateRoleName, tools.UpdateIdentityName,
    tools.HireWorkerName, tools.CreateStreamName, tools.StreamMembersName,
    tools.SubscribeName, tools.UnsubscribeName, tools.InviteWorkersName,
    tools.PublishName, tools.DMName,
}
defaults := append(append([]tool.Name{}, ownerMutationTools...), tools.BaseReadTools...)
```

The end-result set is identical to today's; we just stop duplicating the read names.

### D6 — Merge happens at every Role-creation entry point, not in `orgchart.NewRole`

`orgchart.NewRole` is a pure constructor and should remain so — adding "inject defaults" there would surprise any non-creation caller (tests, fixtures, copies, future re-hydration). The merge happens at each entry point that constructs a *new* Role.

**Entry points covered:**

1. **MCP `create_role` tool** (`tools/create_role.go`) — calls `MergeBaseReadTools(args.Tools)` before `NewRole`.
2. **REST `POST /orgs/{org}/roles`** (`interfaces/server/api/roles.go::createRole`) — same merge, applied to `toToolNames(req.Tools)`. *Discovered during the in-browser demo of the implementation:* the chart UI's "New Role" dialog collects only ID + content (no tools picker) and posts to this REST handler. Without the merge, every Role created through the UI ended up with an empty tool list, so its Workers had no MCP surface at all. The MCP fix alone was insufficient because the UI doesn't go through MCP.
3. **`bootstrap.Run`** — composes the owner default by `append(ownerMutationTools, BaseReadTools...)` directly.
4. **`RoleReconciler.Reconcile`** — uses the same `MergeBaseReadTools` helper at startup to backfill drifted Roles.

`MergeBaseReadTools` is exported from `tools/defaults.go` so all four sites share one implementation. Adding a fifth Role-creation entry point in future is a one-line call to the same helper.

## Data Flow

**On `create_role` call:**

```
caller args.Tools  ─┐
                    ├─►  mergeWithBaseReadTools(...)  ─►  orgchart.NewRole  ─►  Roles.Create
BaseReadTools     ─┘
```

**On API server start:**

```
serve.go  ─►  RoleReconciler.ReconcileAll(ctx)
              └─►  for each org:
                    └─►  Roles.List(orgID)
                          └─►  for each role missing any baseline tool:
                                Roles.Update(role with appended baseline names)
```

**On any subsequent MCP request from a Worker:**

```
unchanged — buildMCPServer reads Worker → Role.Tools live.
The reconciled tools are visible immediately on the next request.
```

## Tradeoffs and Alternatives Considered

- **Alternative: enforce the baseline at MCP-build time** (in `mcp.go`, always inject `BaseReadTools` into the per-request server, regardless of `Role.Tools`). Rejected — it hides the true tool surface from `update_role` callers and from any UI that reads `Role.Tools`, breaking the "Role *is* the capability" invariant from CLAUDE.md.
- **Alternative: make the reconciler a long-running goroutine with a poll interval.** Rejected for v1 — drift is event-driven, not continuous; reconcile-on-startup + reconcile-on-bootstrap covers every known trigger. Trivial to add the loop later if needed.
- **Alternative: validate at `update_role` too** (block removal of baseline tools). Rejected — over-prescriptive; the owner should be allowed to construct an unusual role if they accept the consequences. The reconciler will restore the baseline on the next startup anyway, and we can revisit if this becomes a footgun.

## Test Plan

- **Unit:** `defaults_test.go` — `BaseReadTools` matches an explicit golden list; every name resolves in the tool registry.
- **Unit:** `create_role_test.go` — caller with `tools:[]` gets the full base set; caller with `tools:[publish, managers]` gets the union, `managers` appears once, original order preserved.
- **Unit:** `reconciler_test.go` — reconciler appends missing names; second run is a no-op (no `Roles.Update` call); roles in other orgs are not touched.
- **Integration / regression:** `builtins_test.go` — extend the existing E2E `TestDemoOwnerHiresCEO` (`api/pkg/org/application/tools/builtins_test.go:36-169`) with a §13 assertion: after `r-owner` boots, an MCP `tools/list` on `w-owner` includes both `managers` and `reports`. Add a sibling test that creates a `r-qa-engineer`-like role via `create_role` with `tools:[dm, publish, subscribe, read_events]` and asserts the resulting MCP surface includes the full base read set plus those four.

## Risks and Mitigations

- **Risk:** A future tool gets added to `BaseReadTools` that requires arguments the typical caller cannot supply (e.g. requires an env path). **Mitigation:** the principle in D1 — baseline tools must be safe and useful for *every* role. Reviewers gate additions to the list.
- **Risk:** Reconcile-on-startup runs on a very large org table and slows server boot. **Mitigation:** the reconciler is O(roles); per-role work is a pure in-memory diff with at most one `Roles.Update` write. For realistic org sizes (tens to hundreds of roles per org), this is sub-second. If it ever becomes a concern, gate it behind a config flag.

## Implementation Notes for Future Agents

- Roles are stored via `store.Roles` (GORM). Use `Roles.List(ctx, orgID)` to iterate; use `Roles.Update(ctx, role)` to persist. Both already exist.
- The tool registry (`api/pkg/org/application/tools/registry.go`) is the right place to validate that every name in `BaseReadTools` resolves — do it once in `RegisterBuiltins` as a startup-fail check, so a typo in the defaults file crashes on boot rather than silently producing a role missing tools.
- `r-owner` content is embedded markdown (`bootstrap/templates/owner_role.md`). Do not touch it — only the tool list changes.
- Be careful not to write to `Role.UpdatedAt` when there is nothing to update — tests assert idempotency by checking the row was untouched.
