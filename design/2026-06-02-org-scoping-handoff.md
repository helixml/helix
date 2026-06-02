# helix-org multi-tenant org-scoping refactor — handoff

**Branch:** `refactor/helix-org-redesign`
**Started:** 2026-06-02 evening
**Status:** ~50% complete. Compile-broken at this snapshot — schema + domain + tools + bootstrap done, runtime/handlers/middleware/frontend/tests pending.

This is the multi-tenant refactor of the helix-org subsystem. Every
org_* table now has a composite (id, org_id) primary key with FK to
`organizations(id) ON DELETE CASCADE`, so deleting a helix Org reaps
all its helix-org rows in one Postgres transaction. The URL surface
will move from `/api/v1/org/*` (single-tenant) to
`/api/v1/orgs/{org}/helix-org/*` (per-org), and the React chart moves
from `/helix-org/chart` to `/orgs/:org_id/helix-org/chart`.

## Why I stopped

The refactor's tendrils reach every layer: domain, store, gorm, tools,
runtime, dispatcher, transports, handlers, middleware, server, frontend,
tests. Each signature change (e.g. adding `orgID` to `Spawner`) ripples
into 5-10 more files. I committed the foundation cleanly (two commits)
but pushing further would land a half-finished branch with cascading
compile errors and no Playwright verification.

Better path: this doc + the two foundation commits give you a clean
restart tomorrow.

## Architectural decisions baked in

1. **Composite (id, org_id) primary keys** on every org_* table —
   short readable handles (`w-owner`, `p-root`, `r-owner`) can repeat
   across helix tenants. Store accessors take `orgID` explicitly on
   every Get/List/Update/Delete; Create/Update carry orgID via the
   domain aggregate.
2. **FK on `org_id` to `organizations(id) ON DELETE CASCADE`** —
   Postgres enforces cascade on org hard-delete. Helix's
   `DeleteOrganization` already uses `tx.Unscoped().Delete` for the
   org row, so cascade fires automatically. No app-side cleanup hook
   needed.
3. **Domain constructors require orgID** at New() time — dropped the
   staged `WithOrgID` pattern. Multi-tenant is now a hard requirement,
   not optional scaffolding.
4. **Schema migration via `OpenWithDB(db, Options{ResetSchema: true,
   InstallOrganizationFK: true})`** — first deploy of the multi-tenant
   schema drops + recreates all org_* tables (the composite PK change
   isn't auto-migratable from the prior shape) and installs the FK
   constraint. Tests pass `{ResetSchema: false, InstallOrganizationFK:
   false}` so per-schema test fixtures don't need an `organizations`
   table.
5. **Bootstrap is per-org**, lazy on first chart fetch (not eager on
   helix-org-create). The middleware (TODO) will call
   `bootstrap.Run(ctx, store, Params{OrganizationID: orgID, ...})` on
   first request; subsequent requests fast-path through
   ErrAlreadyInitialised.
6. **URL surface**: `/api/v1/orgs/{org}/helix-org/*` where `{org}`
   accepts slug OR id, resolved via existing
   `HelixAPIServer.lookupOrg`. Frontend mirrors at
   `/orgs/:org_id/helix-org/{chart,workers,settings,streams}`.
7. **MCP gateway**: each helix-org MCP backend is per-org —
   `apiServer.mcpGateway.RegisterBackend("helix-org-{orgID}", ...)` on
   bootstrap, deregistered on org delete.

## Committed (in order)

- `da4bc01bf` — schema foundation: domain types require orgID, store
  interfaces grow orgID, gorm impls have composite PKs + cascade FK
  installer, multitenant_test.go spec written.
- `be4893900` — orgID plumbed through tools + bootstrap + runtime/helix
  state.go. Tools updated: hire_worker, create_position, create_role,
  create_stream, dm, grant_tool, invite_workers, publish, read_events,
  read_grants, read_positions, read_roles, read_streams, read_workers,
  revoke_tool, stream_members, subscribe, unsubscribe, update_identity,
  update_role, worker_log, activation_stream. agent.PublishActivationEvent
  grows orgID.

## TODO — file-by-file punch list

### runtime/helix (currently compile-broken)

- `project.go`: `WorkerProject.Ensure(ctx, orgID, workerID)` — done in
  the WIP, callers (spawner.go ensureProject, helix_org.go) need
  update.
- `spawner.go`: `Spawner` type grew `orgID` parameter (done in
  runtime.go). spawner.go has ~10 `LoadState/SaveProject/SaveSession`
  call sites that need `orgID` threaded through. The orgID flows from
  the Spawner entry point (dispatcher passes it) down through
  ensureProject → ensureSession → bridge.run → publishActivationEvent.
- `workspace.go`: `LoadState`, `SaveSession` call sites grow orgID.
  Public methods on Workspace grow orgID parameter (Sync, MirrorFile,
  EnsureBranch, WriteOrgFile). Use the WorkspaceSync interface to
  thread orgID through.
- `hire.go`: `Hire.OnHire(ctx, orgID, workerID, hiringUserID)` —
  signature updated in runtime.HireHook interface (done). Impl needs
  the orgID parameter and pass to SaveHiringUser.

### lifecycle (small, compile-broken)

- `lifecycle.go`: `Service.Fire(ctx, orgID, workerID)` — orgID
  parameter, pass to all store calls (Workers.Get, Subscriptions.*,
  Grants.*, Environments.*, Workers.Delete, helix.LoadState, etc.).
  Owner-protect changes from comparing IDs to comparing (orgID,
  ownerID) pair — but since the embedded owner ID is now per-org
  (`w-owner` repeated per tenant), the protection collapses to "refuse
  to fire any w-owner".

### dispatcher.go

- `Dispatcher` constructor takes nothing org-specific today. The
  per-event orgID comes from the event itself (`event.OrganizationID`).
  Update `dispatch.Dispatcher.Dispatch(ctx, ev)` to pull orgID from
  `ev.OrganizationID`, scope all subsequent store calls, and pass orgID
  to the spawner.

### server/mcp.go (org MCP backend)

- Read orgID from `inv.Caller.OrganizationID()` (Caller is now
  guaranteed to have orgID). Pass to Workers.Get and Grants.ListByWorker.

### server/webhook.go

- Webhook URL is `/webhooks/{id}` where `{id}` is a stream ID. Stream
  IDs are NOT globally unique under composite PK — we need to either:
  (a) include orgID in the URL: `/webhooks/{org}/{streamID}`, or
  (b) keep stream IDs globally unique via a UUID `s-<ulid>` form
  alongside the readable name.
  RECOMMENDATION: (a) — embed orgID in the webhook URL. Stream
  references in transport configs become `(orgID, streamID)` pairs.

### transports/{github,postmark}.go

- Streams.List() needs orgID. The transport runs as a goroutine —
  where does orgID come from? Each transport instance is registered
  per-org (helix_org.go will iterate orgs that have helix-org enabled
  on startup). Pass orgID to NewGitHubTransport / NewPostmarkTransport
  constructors.

### server/api/api.go (15 handlers)

- Replace `a.deps.Owner` with `OwnerFromContext(r.Context())` — a new
  helper that reads orgID + ownerID from request context, set by the
  middleware. All Get/List/Update calls take orgID from context.
- Specific handlers needing update:
  - `getChart` → Positions.List(orgID), Workers.List(orgID)
  - `listPositions` → Positions.List(orgID)
  - `listRoles` → Roles.List(orgID)
  - `listWorkers` / `getWorker` → Workers.List/Get(orgID)
  - `updateWorkerIdentity` / `updateWorkerRole` → Workers.Get(orgID)
  - `listSettings` / `setSetting` / `deleteSetting` → Configs(orgID)
  - `listStreams` / `streamEventsSSE` / `publishToStream` →
    Streams/Events/Subscriptions(orgID)
  - `hireWorker` → orgID into the synthetic Invocation
  - `fireWorker` → Lifecycle.Fire(orgID, workerID)

### server/middleware

- New file `api/pkg/server/helix_org_middleware.go`:
  ```go
  func (s *HelixAPIServer) withHelixOrgScope(next http.Handler) http.Handler {
      return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          orgSlugOrID := mux.Vars(r)["org"]
          org, herr := s.lookupOrg(r.Context(), orgSlugOrID)
          if herr != nil { /* 404 */ return }
          // authorize membership
          user := getRequestUser(r)
          if !s.isOrgMember(r.Context(), user, org.ID) { /* 403 */ return }
          // ensure helix-org bootstrap for this org
          if err := s.ensureHelixOrgBootstrap(r.Context(), org.ID); err != nil { /* 500 */ }
          ctx := WithHelixOrgID(r.Context(), org.ID)
          next.ServeHTTP(w, r.WithContext(ctx))
      })
  }
  ```

### server/helix_org.go (major rewrite)

- Replace the singleton `apiDeps.Owner = "w-owner"` with per-request
  resolution: handlers read orgID from ctx, then the owner Worker is
  `(orgID, "w-owner")`.
- `lazyHelixOrgSpawner` becomes per-org or wraps an orgID-passing
  closure.
- The MCP backend registration moves from "once at startup" to
  "once per org bootstrap" — when an org bootstraps, register
  `helix-org-{orgID}` backend on the MCP gateway.
- Remove `bootstrap.Run` from the init path; replace with
  `ensureHelixOrgBootstrap(ctx, orgID)` callable from middleware.

### server.go

- Replace `authRouter.PathPrefix("/org/").Handler(...)` with
  `authRouter.PathPrefix("/orgs/{org}/helix-org/").Handler(
       requireFeature(alphaFeatureHelixOrg)(
           withHelixOrgScope(
               http.StripPrefix(APIPrefix+"/orgs/"+orgVar+"/helix-org", orgHandlers.api))))`.

### Frontend

- `frontend/src/router.tsx`: move 5 helix-org routes under
  `/orgs/:org_id/helix-org/{chart,workers,workers/:worker_id,settings,streams}`.
  Path-encode the org segment.
- `frontend/src/services/helixOrgService.ts`: replace const
  `BASE = '/api/v1/org'` with a `useHelixOrgBase()` hook that reads
  the current `:org_id` from `useRouter()` and returns
  `/api/v1/orgs/${orgID}/helix-org`. Every useQuery + useMutation in
  the file uses the hook.
- `frontend/src/pages/HelixOrgChart.tsx`: `useRouter` already gives
  `:org_id`; pass to the service hooks. The React Flow chart code
  itself doesn't need org-awareness (orgID is implicit in the data).
- Sidebar nav: add a "Helix Org" entry under the existing org section
  (under Orgs.tsx) so the chart is discoverable from the org page,
  not just by direct URL.

### Tests (~50 files, mostly mechanical)

The pattern in every test file is:
- Replace `role.New(id, content, nil, nil, now)` with
  `role.New(id, content, nil, nil, now, "org-test")`
- Same for NewPosition, NewHumanWorker, NewAIWorker, NewSubscription,
  NewToolGrant, NewEvent, NewEnvironment, NewConfig, NewMessageEvent,
  NewStream, activation.New
- Store calls grow `"org-test"` as the second argument:
  `Workers.Get(ctx, "w-foo")` → `Workers.Get(ctx, "org-test", "w-foo")`

A regex pass might catch most of these. Files affected (from earlier
exploration):
- api/pkg/org/domain/*_test.go (8 files)
- api/pkg/org/role/role_test.go
- api/pkg/org/store/gorm/*_test.go (4 files)
- api/pkg/org/bootstrap/bootstrap_test.go
- api/pkg/org/dispatch/dispatcher_test.go
- api/pkg/org/runtime/helix/*_test.go (5 files)
- api/pkg/org/server/api/api_test.go
- api/pkg/org/server/server_test.go
- api/pkg/org/server/webhook_test.go
- api/pkg/org/transports/{github,postmark}/*_test.go (2 files)
- api/pkg/org/tools/*_test.go (5 files)
- api/pkg/org/activation/activation_test.go

The store-test fakeRepo at activation/activation_test.go:160 needs
its Repository method signatures updated to match the new interface.

### Playwright verification scenarios

1. **Two orgs, isolated charts**:
   - Log in as user A. Org X exists. Navigate to
     `/orgs/X/helix-org/chart`. Verify chart shows
     `w-owner@X / p-root@X / r-owner@X`.
   - Same user (different tab) navigates to `/orgs/Y/helix-org/chart`.
     Verify chart shows `w-owner@Y / p-root@Y / r-owner@Y`.
   - Hire `w-mark` in org X via the chart. Verify it appears in X but
     NOT in Y.
   - Fire `w-mark` in X. Verify gone.

2. **Cascade on org delete**:
   - In org X, hire workers, create a stream, publish events. Capture
     row counts in Postgres for org_workers / org_positions /
     org_streams / org_events for org_id = X.
   - DELETE /api/v1/organizations/X (helix's existing endpoint).
   - Verify row counts for org_id = X are now 0 — the FK cascade fired.
   - Verify rows for org_id = Y still exist (cross-tenant isolation).

3. **Membership authz**:
   - User B (not in org X) tries `GET /api/v1/orgs/X/helix-org/chart`.
     Expect 403.

## Risk surface

- The composite PK migration drops the existing alpha tables on the
  first deploy. There are ~3-5 test users with helix-org alpha data
  (mostly demo data); confirm with phil@winder.ai that wiping is OK
  before deploying to the staging cluster. (Local dev: always fine.)
- The FK install runs raw ALTER TABLE — it's idempotent (DO $$ ... IF
  NOT EXISTS) but the SQL is Postgres-specific. SQLite test paths
  (none currently — sqlite was dropped in 5dcf15d38) wouldn't see it.
- The owner protection in lifecycle.Fire previously matched on `id ==
  "w-owner"` (singleton). Under composite PK, every org has its own
  `w-owner`. Protection should compare `id == "w-owner"` regardless of
  orgID — the rule "you can never delete the owner Worker" stays
  global.

## Open questions for tomorrow

1. **Webhook URL shape**: `/webhooks/{streamID}` no longer disambiguates
   under composite PK. Two options: (a) embed orgID in URL
   `/webhooks/{org}/{streamID}`; (b) generate globally unique synthetic
   stream IDs (UUID-based) for webhook streams specifically.
   RECOMMENDATION: (a) for symmetry with the rest of the helix-org
   surface.
2. **MCP gateway backend naming**: `RegisterBackend("helix-org-{orgID}",
   ...)` per org, or one backend that demuxes by URL? Per-org cleaner
   but adds gateway state; one backend simpler but each MCP request
   needs orgID in the URL. RECOMMENDATION: per-org backend.
3. **Org-create eager bootstrap vs lazy**: middleware-on-first-fetch is
   simpler now; future enhancement could hook helix's `createOrg`
   handler to bootstrap eagerly so the chart loads instantly on first
   visit. Skip for v1.

## Resume command

When you /resume tomorrow:
```
cd ~/helix
git log -3 --oneline   # confirm da4bc01bf and be4893900 are at HEAD~1, HEAD~2
cat design/2026-06-02-org-scoping-handoff.md   # this file
# Then start with runtime/helix project.go callers — that's the next
# closest-to-compiling target.
```
