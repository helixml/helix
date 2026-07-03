# Design: Unify Bot Creation Flow So Slack Auto-Router Reconciles MCP-Created Bots

## Summary

The Slack auto-router reconcile logic already lives in the right place —
`lifecycle.Create` calls `runOrgReconcilers` for every create/delete
(`api/pkg/org/application/lifecycle/lifecycle.go:243,366`). The defect is purely
that the **MCP** surface holds a *different* `lifecycle.Service` instance whose
`OrgReconcilers` slice is empty. The fix is to make the REST and MCP surfaces
share **one** `lifecycle.Service` (the one with the `slackrouting` reconciler
wired), eliminating the drift structurally.

## Current wiring (the two instances)

```
                       lifecycle.Create() ── runOrgReconcilers(OrgReconcilers)
                              ▲                         │
        ┌─────────────────────┴───────────┐            │ iterates the slice
   REST instance                     MCP instance       ▼
   helix_org.go:669                  builtins.go:221   REST: [slackRouteReconciler]  ✅
   OrgReconcilers = [slackRouter]    OrgReconcilers = nil   MCP:  []                  ❌
   (set at helix_org.go:776)         (no Config field for it)
        ▲                                   ▲
   apiDeps.Lifecycle (bots.go)         deps.Build() @ helix_org.go:642
   POST /bots                          create_bot tool (create_bot.go:101)
```

`deps.Build()` (line 642) constructs the MCP tools **before** `slackRouteReconciler`
exists (line 769), so the MCP instance can never receive it as written.

## Chosen approach — single shared `lifecycle.Service`

Build the reconciler-complete `lifecycle.Service` **once** in the composition
root and inject the same pointer into both the REST `apiDeps.Lifecycle` and the
MCP tool `Deps.Lifecycle`. This is the "one implementation, no drift" outcome the
codebase already aims for (see the comment at `lifecycle.go:7-11`).

### Mechanism

1. **Add an injection seam to `mcptools`.** Give `mcptools.Config` (and/or the
   `Build()` path in `api/pkg/org/interfaces/mcptools/builtins.go`) an optional
   pre-built `Lifecycle *lifecycle.Service`. When set, `Config.Build()` uses it
   verbatim instead of calling `lifecycleService()` to construct a fresh one.
   When unset (tests, standalone runtimes), `lifecycleService()` keeps its
   current behaviour so nothing else breaks.

2. **Reorder the composition root** (`api/pkg/server/helix_org.go`) so the shared
   `lifecycleSvc` is fully assembled — including
   `OrgReconcilers = [slackRouteReconciler]`, plus `Bots`, `Subscriber`,
   `Dispatcher`, `Helix`, `Mirror` — **before** the MCP registry is built. Then
   set `deps.Lifecycle = lifecycleSvc` and call `deps.Build()`. Concretely, the
   `buildOrgServices(...)` call (currently line 710), the `slackRouteReconciler`
   construction (line 769), and the `lifecycleSvc` assembly (line 669) move above
   the `mcptools.RegisterBuiltins(reg, deps.Build())` call (line 642). The
   `svc.Bots` / `svc.Subscriptions` / `svc.Processors` dependencies are already
   satisfied by `buildOrgServices` and do not depend on `reg`, so the reorder is
   safe. Verify no other line between 642 and 776 depends on `reg` being built
   first.

3. **Delete the now-redundant second construction.** After injection, the MCP
   path no longer builds its own lifecycle service in production; both `apiDeps`
   and the MCP `Deps` reference `lifecycleSvc`.

### Why not just "add OrgReconcilers to mcptools.Config and wire it too"

That would fix the reported symptom but leave two instances that must be kept in
sync forever (the exact failure mode we just hit — and it would still diverge on
Dispatcher/Helix/Mirror). A single shared instance makes drift impossible, which
is what the user asked for ("unify the code so there are no differences").

## Files touched

| File | Change |
|------|--------|
| `api/pkg/org/interfaces/mcptools/builtins.go` | Add optional injected `Lifecycle` to `Config`; `Build()`/`lifecycleService()` prefer it when set. |
| `api/pkg/server/helix_org.go` | Reorder so the reconciler-complete `lifecycleSvc` is built before `deps.Build()`; inject it into the MCP `Config`; drop the duplicate MCP lifecycle construction. |
| `api/pkg/org/interfaces/mcptools/*_test.go` (new/edited) | TDD red test (see below). |

No changes to `lifecycle.Create`, `slackrouting.Reconciler`, or `create_bot`'s
public contract — the business logic stays in the application layer.

## TDD red test

**Goal:** a test that is **red on current HEAD** and green after the fix, proving
an MCP-created bot triggers the whole-org reconcilers.

Recommended (behavioural, `mcptools` package):
- Build the MCP tool `Deps` the way production does — via `mcptools.Config` over
  an in-memory store (`api/pkg/org/infrastructure/persistence/memory`) — with a
  **spy `OrgReconciler`** (a tiny fake recording `Reconcile(orgID)` calls)
  injected through the new `Config.Lifecycle` seam.
- Register builtins, invoke the `create_bot` tool for a valid bot.
- Assert the spy's `Reconcile` was called exactly once with the org id.

On current HEAD the spy cannot be wired (no seam) / is never invoked → the test
fails, capturing the bug. After the fix it passes. Follow the existing patterns
in `api/pkg/org/application/lifecycle/hire_test.go` (in-memory store + bots
service) and `api/pkg/org/application/slackrouting/reconciler_test.go`.

Optional complementary test (`server` package): assert `apiDeps.Lifecycle` and
the MCP registry's create_bot `Deps.Lifecycle` are the same instance (or both
have non-empty `OrgReconcilers`) — a structural guard against future re-splitting.

## Manual verification (localhost:8080)

1. Bring up the inner stack; register `test@helix.ml` / `helixtest`, complete
   onboarding.
2. Ensure an Automated Slack router exists (connect a Slack workspace, or seed
   the Automated filter processor).
3. Create a bot **via MCP/chat** (`create_bot`) and note its id.
4. Confirm a managed route + subscription appeared:
   - processor `Outputs` contains one with `ManagedFor == <botID>`;
   - a subscription row links the bot to that output topic;
   - API logs show `slackrouting: added route for bot`.
5. Repeat via the **UI** `POST /bots` and confirm identical behaviour.
6. Check DB, e.g.:
   `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT ... FROM org_subscriptions WHERE ...;"`

## Risks / notes

- **Composition-root reorder** is the main risk — `helix_org.go` is a large
  wiring function. Move only the three blocks named above and confirm the Go
  build passes; nothing between them should depend on `reg`.
- Single-replica assumption for socket-mode Slack is unchanged.
- The auto-router remains a correct no-op when no Automated router exists — the
  fix only guarantees the reconcile *runs* on MCP creates.

---

## Implementation Notes (as-built)

**Seam chosen:** Added an optional `Lifecycle *lifecycle.Service` field to
`mcptools.Config` (`api/pkg/org/interfaces/mcptools/builtins.go`).
`Config.lifecycleService()` now returns `c.Lifecycle` verbatim when set, else
falls back to the previous standalone construction. This keeps every existing
test/runtime that uses `DefaultDeps(...).Build()` working unchanged, while
letting the composition root inject the shared instance.

**Reorder (as-built):** Rather than move the large `buildOrgServices` block up,
I moved the *small* MCP registry block **down**. In
`api/pkg/server/helix_org.go`:
- Removed `reg := mcptools.NewRegistry()` / `RegisterBuiltins(reg, deps.Build())`
  / `orgServer := ...` from their old spot (~line 641-657); left `promptReg`
  where it was and added a NOTE comment there.
- Re-added that block immediately after
  `lifecycleSvc.OrgReconcilers = append(..., slackRouteReconciler)`, preceded by
  `deps.Lifecycle = lifecycleSvc`.
- Safe because `reg` and `orgServer` are only consumed much later
  (`apiDeps.Tools = reg`, `orgServer.Handler(...)`), and `buildOrgServices`
  takes `deps` by value and never reads `Lifecycle`.

**Same-instance guarantee:** `apiDeps.Lifecycle = lifecycleSvc` (unchanged) and
`deps.Lifecycle = lifecycleSvc` → `deps.Build()` → `lifecycleService()` returns
that same pointer. So REST `POST /bots` and MCP `create_bot` now drive the exact
same `*lifecycle.Service`, including its `OrgReconcilers` (Slack auto-router),
`Dispatcher`, `Helix`, and `Mirror`.

**Base-tool parity confirmed:** `buildOrgServices` builds `svc.Bots` with
`BaseTools: mcptools.BaseReadTools` (helix_org.go:224) — the same baseline the
MCP path used — so sharing the lifecycle preserves the base-read-tool union for
MCP-created bots.

**Red→green:** `create_bot_slackrouting_test.go` failed to compile on HEAD
(`deps.Lifecycle undefined`); after the seam + reorder it passes. Full
`./pkg/org/...` suite (38 packages) green.

**Files changed (helix):**
- `api/pkg/org/interfaces/mcptools/builtins.go` — `Config.Lifecycle` seam.
- `api/pkg/server/helix_org.go` — reorder + inject shared lifecycle.
- `api/pkg/org/interfaces/mcptools/create_bot_slackrouting_test.go` — regression test (new).
