# Cross-tenant leak: helix-org spawner freezes the first org's identity

**Date:** 2026-06-09
**Branch:** `fix/org-multitenancy-leak` (off `main`)
**Severity:** Critical — cross-tenant data write. Roles/workers created by one
org's owner land in a *different* org.

## Symptom (reported)

> Created a new org, configured the Helix Org feature for it, asked the owner
> of that org to create a new role and hire a worker. The role and worker were
> created in a **different** org.

## Root cause

`lazyHelixOrgSpawner` (`api/pkg/server/helix_org.go`) caches **one**
process-wide `runtime.Spawner`, built from the **first** org that ever
activates a worker:

```go
var spawner runtime.Spawner            // package-lifetime singleton
...
if current == nil {
    cfgVal, _ := buildHelixOrgSpawnerConfig(ctx, orgID, ...) // orgID = FIRST org
    spawner = runtimehelix.Spawner(cfgVal)                   // OrgID + HelixOrgURL frozen
}
return current(ctx, orgID, workerID, ...)                    // reused for EVERY org
```

`buildHelixOrgSpawnerConfig` bakes two **tenant-specific** values into that
frozen config:

- `SpawnerConfig.OrgID = orgID`
- `SpawnerConfig.HelixOrgURL = baseURL + "/api/v1/mcp/helix-org/" + orgID`

Every later activation, for **any** org, reuses the first org's frozen config.
The frozen values are then actively used (they are NOT "dead weight" as the old
comment claimed):

1. `SpawnerConfig.ensureProject` (`spawner.go`) builds
   `WorkerProject{OrgID: c.OrgID, HelixOrgURL: c.HelixOrgURL}` — first org.
2. `WorkerProject.Ensure` (`project.go:176`) sends
   `applyReq.OrganizationID = a.OrgID` and (`:236`) writes
   `HELIX_ORG_URL = a.HelixOrgURL` as a project secret — first org.
3. `SpawnerConfig.ensureHelixOrgMCP` (`spawner.go:381`) calls
   `AttachHelixOrgMCP(c.HelixOrgURL, ...)` — first org. **This is the kill
   shot.** It runs on *every* activation and re-attaches the worker's
   helix-org MCP entry pointing at the **first org's** gateway URL.

## Attack path (matches the report exactly)

1. Owner clicks *Start Desktop* → `apiHandler.activateWorker`.
2. Step 1 `dynamicProjectApplier.Ensure(orgB, w-owner)` is **correct** — it
   rebuilds per-org and attaches the MCP at orgB's URL.
3. Step 4 `Dispatcher.DispatchManual(orgB, ...)` → `lazyHelixOrgSpawner`.
4. The cached inner spawner (frozen to orgA, the first org to ever activate)
   runs `ensureHelixOrgMCP` → re-attaches w-owner's MCP at
   `/api/v1/mcp/helix-org/<orgA>`, **clobbering** the correct orgB entry.
5. orgB's owner desktop boots Zed, reads the agent-app config → MCP URL = orgA.
6. Owner chats "create a role / hire a worker" → MCP call hits
   `HelixOrgMCPBackend` with orgA in the path → `lookupOrg` resolves orgA →
   `create_role` / `hire_worker` write into **orgA**.

The MCP gateway, store layer, and tools are all correctly org-scoped (they read
`org` from the request URL and use composite `(org_id, id)` PKs). The leak is
purely that the worker's MCP URL is stamped with the wrong org by the frozen
spawner.

## Fix

Make the spawner genuinely multi-tenant. The host wrapper is the only place
with per-org config (the `configregistry.Registry`), so it must build the
SpawnerConfig **per activation**, scoped to the activation's org — never cache
one org's config and reuse it.

1. `lazyHelixOrgSpawner`: drop the frozen-singleton cache. Build a fresh
   per-org `SpawnerConfig` on every activation. Preserve the one thing the
   cache legitimately provided — a single process-wide inflight cap — by
   creating one shared semaphore and injecting it into each per-activation
   config.
2. `SpawnerConfig.Sem` (new, optional): when set, `Spawner()` uses it instead
   of minting its own. Lets the host share one global semaphore across all
   per-org spawner instances.
3. `WorkerProject.Ensure`: use the `orgID` **parameter** for
   `applyReq.OrganizationID`, not the struct field `a.OrgID`. The function is
   handed an `orgID`; mixing it with a struct field was the latent
   inconsistency that let the frozen field leak through. Defence in depth.

Worker.* drift handling is unaffected: `dynamicProjectApplier.Ensure` still
runs first and re-reads `worker.*` per activation; rebuilding the spawner
config per activation just means its own copy of those values is now also
current instead of frozen.

## Two further leaks found during the review

The brief was "review the org-based multi-tenancy," so I swept for other
process-wide state that should be per-tenant. Two more real leaks:

### 2. Activation queue lane keyed by workerID alone (CRITICAL — second
root cause of the same symptom)

`api/pkg/org/domain/activation/queue.go`. The dispatcher holds one
process-wide `Queue`; its serialisation lanes were keyed by `workerID`:

```go
lanes sync.Map // map[orgchart.WorkerID]*workerLane
func (q *Queue) laneFor(workerID orgchart.WorkerID) *workerLane { ... }
```

Worker IDs are unique only within an org — **every** org's owner is
`w-owner`. So all orgs' `w-owner` shared one lane: a second org's `Enqueue`
folded its trigger into the first's `pending` and overwrote
`lane.orgID` (last-writer-wins, queue.go:80). The lane runner then read that
overwritten `orgID` (queue.go:105) and ran one org's activation under another
org's context — wrong project, MCP URL, secrets. This independently produces
the reported symptom.

**Fix:** key lanes by `laneKey{orgID, workerID}`.

### 3. Streamhub wake topics not org-scoped (LOW — spurious wakes, not a
data leak)

`api/pkg/org/application/streamhub/streamhub.go`. Wake topics were
`helix-org.stream-updates.<streamID>`. Stream IDs collide across orgs
(`s-activations-w-owner`, named streams like `s-general`), so one org's
`Notify` woke another org's subscribers on a colliding id. The hub is
**wake-only** (nil payload; subscribers re-query their own org-scoped store),
so this leaks no data — the impact is spurious cross-org wakeups. Fixed for
correctness anyway: topics are now `…stream-updates.<orgID>.<streamID>` and
`SubscribeAll` uses a per-org wildcard. `Notify`/`Subscribe`/`SubscribeAll`
gained an `orgID` parameter; all ~10 call sites pass their operating org.

### 4. Transcript Mirror trackers keyed by workerID alone (found on rebase)

`api/pkg/org/infrastructure/runtime/helix/mirror.go`. The session-layer
`Mirror` (added by the `session-layer transcript mirror` feature on main) is a
process-wide singleton whose `tracked map[orgchart.WorkerID]context.CancelFunc`
was keyed by workerID. Identical defect class to #2: every org's `w-owner`
collapses into one entry, so `Ensure(orgB, "w-owner")` sees the slot occupied
and silently no-ops (orgB's transcript never mirrors), and `Stop` cancels the
wrong tenant. Fixed by keying `mirrorKey{orgID, workerID}`; `Stop` now takes
`orgID`.

This one is instructive: the bug class (org-blind singleton keyed by a
non-unique id) **reappeared in brand-new code** the moment a singleton was
added, which is the basis for the systematic-audit recommendation below.

## Regression tests

- `project_test.go::TestEnsureScopesProjectToParamOrg_NotStructOrgID` — Ensure
  scopes the project to its `orgID` arg, not the struct's frozen `OrgID`.
- `spawner_test.go::TestSpawnerHonorsSharedSemaphore` — the inner spawner uses
  the host-supplied shared `Sem` (the global cap that replaced the cache).
- `queue_test.go::TestQueueIsolatesSameWorkerIDAcrossOrgs` — two orgs' `w-owner`
  run in parallel under their own org, not one shared lane.
- `streamhub_test.go::TestNotify_IsolatedAcrossOrgs` /
  `TestSubscribeAll_IsolatedAcrossOrgs` — one org's Notify never wakes another
  org's subscriber on a colliding stream id.

## Not fixed (verified correct during the sweep)

MCP gateway org resolution (`mcp_backend_helix_org.go` reads `org` from the
URL, `lookupOrg`, `authorizeOrgMember`), the config registry, the tool
registry, store lookups (composite `(org_id, id)` PKs), the in-proc client
(service-account scoped; `ApplyProject` honours `req.OrganizationID`), and the
Workspace (org-agnostic; methods take `orgID` per call).
