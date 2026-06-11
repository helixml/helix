# helix-org: Application-Service Layer Review & Refactor Plan

**Date:** 2026-06-10
**Author:** review pass over `api/pkg/org/**` + `api/pkg/server/helix_org*.go`
**Status:** proposal / design — no code changed yet

## 1. Scope & method

This reviews the DDD layering of the embedded helix-org module and the
host-side glue that instantiates it. I read every handler in
`api/pkg/org/interfaces/server/**`, every package in
`api/pkg/org/application/**`, the domain store aggregate, and all
`api/pkg/server/helix_org*.go` files. Findings below cite `file:line`.

The two complaints that motivated this are both real and both reduce to
one root cause: **there is no application-service seam.** Use cases live
either inline in interface adapters (REST handlers, MCP tools) or are
smeared across the composition root. Concretely:

1. Interface adapters reach straight into the domain repositories
   (`store.Store.Workers/Roles/Streams/...`) instead of calling an
   application service that owns the use case.
2. `initHelixOrgHandler` (`helix_org.go`, 1020 lines) is a composition
   root that has absorbed real business logic (GitHub
   identity/installation/repo aggregation as inline closures; API-key
   provisioning leaked into the `server` package).

## 2. What's actually there (credit where due)

The module is *already* laid out in DDD quadrants and that layout is
sound — this is a refactor, not a rewrite:

```
api/pkg/org/
  domain/         orgchart, streaming, activation, transport, store (repo interfaces), tool, config, principal, environment
  application/    dispatch, lifecycle, topology, bootstrap, configregistry, streamhub, prompts, tools, agent
  infrastructure/ persistence/{gorm,memory}, runtime/helix, streamcron, transports/{github,postmark}
  interfaces/     server (MCP + webhook), server/api (REST/JSON)
```

The domain is clean. The store exposes repository interfaces — `Roles`,
`Workers`, `ReportingLines`, `WorkerRuntimeState`, `Streams`,
`Subscriptions`, `Events`, `Environments`, `Configs`, `Activations`
(`domain/store/store.go:181-192`). Three genuine application services
already exist and are correct:

- **`dispatch.Dispatcher`** (`application/dispatch/dispatcher.go:60`) —
  event fan-out to subscribed Workers + outbound transports.
- **`lifecycle.Service`** (`application/lifecycle/lifecycle.go:39`) —
  cross-cutting `Fire` / `DeleteRole` teardown (Helix project+app
  deletion, store cleanup, env-dir removal, topology reconcile, mirror
  stop).
- **`topology.Reconciler`** (`application/topology/reconciler.go:21`) —
  converges activation/team/DM Streams from the reporting graph.

These three are the model to follow. The problem is everything *else* —
the structural CRUD use cases — never got the same treatment.

## 3. Problem A — no application-service seam between adapters and the store

### 3.1 The smoking gun: the same use case implemented twice

`create_role` exists as **two independent implementations** that both
build the aggregate and write the repository directly:

- MCP tool — `application/tools/create_role.go:60-66`
  ```go
  r, err := orgchart.NewRole(id, args.Content, args.Tools, args.Streams, t.deps.Now(), orgID)
  if err != nil { return nil, err }
  if err := t.deps.Store.Roles.Create(ctx, r); err != nil { return nil, err }
  ```
- REST handler — `interfaces/server/api/roles.go:51-59`
  ```go
  rl, err := orgchart.NewRole(orgchart.RoleID(strings.TrimSpace(req.ID)), req.Content, tools, streams, a.deps.Now().UTC(), orgID)
  if err != nil { writeError(...); return }
  if err := a.deps.Store.Roles.Create(r.Context(), rl); err != nil { writeError(...); return }
  ```

Two adapters, one use case, zero shared code. They *can and will* drift —
the day someone decides creating a role should (say) reconcile topology
or emit an audit event, they'll fix one path and forget the other.

### 3.2 The codebase already knows the right answer — and applies it inconsistently

The fix pattern is present for exactly one use case. `hire_worker` is
implemented once (the `tools.HireWorker` tool) and the REST handler
*delegates to it*. The `Deps.HireWorker` comment says so explicitly
(`interfaces/server/api/api.go:89-94`):

> "The REST POST /workers handler builds a synthetic Invocation around
> the owner Worker and dispatches through this same path so REST hires
> and chat-driven hires produce identical store state."

Likewise `deleteRole` → `lifecycle.DeleteRole` (`roles.go:174`) and
`fireWorker` → `lifecycle.Fire`. So the module has the right instinct in
three places and ignores it everywhere else.

### 3.3 The everywhere-else inventory

`interfaces/server/api/Deps` holds `Store *store.Store` directly
(`api.go:79`) alongside the application services, which is what lets
handlers take the shortcut. Direct repository access from interface
adapters (representative, not exhaustive — ~32 of ~38 handlers do this):

| Use case | Interface adapter (direct store) | Existing app service? |
|---|---|---|
| Create role | `roles.go:56` | `tools.CreateRole` (duplicated, not reused) |
| Update role | `roles.go:121,140` | `tools.UpdateRole` (duplicated) |
| Update worker role | `api.go:1075,1085,1091` | `tools.UpdateRole`-ish (duplicated) |
| Update worker identity | `api.go:865,870` | `tools.UpdateIdentity` (duplicated) |
| Create stream | `api.go:1351` | `tools.CreateStream` (duplicated) |
| Update/delete stream | `api.go:1525,1585` | — (no service at all) |
| Subscribe / unsubscribe | `api.go:1864,1880,1913` | `tools.Subscribe/Unsubscribe` (duplicated) |
| Publish to stream | `api.go:1767` + `Hub.Notify` + `Dispatch` | `tools.Publish` (duplicated) |
| Add / remove parent | `api.go:947,972,1020` (+ `Topology.Reconcile`) | — (orchestration inline in handler) |
| List workers / overview | `api.go:298,303,415,424,443` | `tools.ListWorkers` (duplicated) |
| List/get streams + events | `api.go:1234,1259,1269,1388,1409,1416` | `tools.ListStreams/GetStream` (duplicated) |

The MCP side has the mirror-image problem: the per-Worker MCP bootstrap
reads `store.Workers.Get` / `store.Roles.Get` directly
(`interfaces/server/mcp.go:80,86`), and the webhook handler appends
events directly (`interfaces/server/webhook.go:53,94`).

### 3.4 The "tools" coupling that blocks naive reuse

You can't just point every REST handler at the matching tool, because
`tool.Tool` is MCP-shaped: `Invoke(ctx, Invocation) (json.RawMessage,
error)` where `Invocation{Caller tool.Worker, Args json.RawMessage}`
(`domain/tool/tool.go`). Reusing a tool from REST means fabricating a
synthetic `Caller` and round-tripping args through JSON — which is
exactly the awkwardness `hireWorker` absorbs today. That coupling is the
real signal: **the use-case logic should not live inside an MCP
protocol adapter.** It should live in a typed application service that
the MCP tool and the REST handler both call.

### 3.5 Honest scoping — reads vs. writes

I am *not* claiming every `Store.X.List` in a read handler is a sin.
Wrapping a pure DTO projection (`listStreams`, `getOverview`) in a
service buys little beyond consistency. The high-value targets are the
**mutations and orchestrations**, where invariants, ordering, and
side-effects (topology reconcile, hub notify, dispatch, audit row) must
not diverge between callers. I'd treat read-model wrapping as optional
polish (a thin query service for consistency), and the write seam as
mandatory.

## 4. Problem B — the composition root has absorbed business logic

`initHelixOrgHandler` (`helix_org.go:116-634`) is doing far more than
wiring. Legitimate composition (open store, build registries, construct
dispatcher/lifecycle/spawner, mount routes) is tangled with real logic:

- **GitHub installation status** — `helix_org.go:403-478`: ~75 lines of
  inline closure that verifies app installs against GitHub, syncs
  installation ids, deletes stale `ServiceConnection` rows, backfills
  owners, and builds install/manage URLs. That is a GitHub-integration
  service, not wiring.
- **GitHub repo aggregation** — `helix_org.go:483-532`: ~50 lines
  minting per-installation tokens and listing repos across installations.
- **API-key provisioning leaked into `server`** — `ensureHelixOrgServiceAPIKey`
  and `resolveUserHelixAPIKey` (`helix_org_chat.go:88-191`) read/write
  the **host** store's `users` / `api_keys` (grant alpha flag, mint
  keys). This is credential-provisioning business logic sitting in the
  composition file, reaching into `helixStore` directly.

By contrast, the adapters that *are* done right show the target shape:
`helix_org_inproc.go` (port impl for `ProjectService`/`SpawnerClient`),
`helix_org_github.go` (`newGitHubOAuthResolver` /
`newOrgGitHubIdentityResolver` — host state injected as closures, org
module never imports `oauth`/`store`), and `helix_org_github_manifest.go`
(manifest flow behind injected `getKey`/`st`). These prove the boundary
*can* be clean; the inline closures in `helix_org.go` just never got
extracted to join them.

Cross-layer check (good news): no host handler *outside* the
`helix_org_*.go` files imports `org/domain`, `org/infrastructure`, or
`org/application`. The leak is contained to the glue files — so fixing
it is local.

## 5. Target design

### 5.0 Design conventions for the new code

The new services follow a specific OO style. Where it collides with a
strong Go convention, we follow Go and note the deviation inline.

- **Name classes by what they are, not what they do — no `-er` suffix.**
  The per-aggregate services are plural nouns: `Roles`, `Workers`,
  `Streams`, `Subscriptions`. A `roles.Roles` type *is* the roles
  collection; it isn't a "RoleManager"/"RoleService". (The existing
  `Dispatcher`/`Reconciler` keep their names — renaming churns working
  code for no behaviour gain, and `-er` is idiomatic for the small Go
  *interfaces* they're consumed through. New code avoids `-er` for
  concrete types.)
- **Methods are builders (noun) or manipulators (verb), rarely both.**
  `Streams.Create(...)` returns the new stream; `Streams.Delete(...)`
  returns only an error. Don't return data from a mutator that also
  has side effects beyond the obvious.
- **Tell, don't ask.** This is the core argument for the whole layer.
  Today handlers *ask*: `Get` the role, mutate exported fields, `Update`
  it (`roles.go:121-144`). Instead *tell* the object: `roles.Update(ctx,
  orgID, id, patch)` and the service owns the read-modify-write +
  invariants. The store stops being a public data bag that every adapter
  pokes at.
- **Immutability + richer encapsulation over getters/setters.** Mutations
  go through `With*` builders on the aggregate (the pattern already
  exists — `Worker.WithIdentityContent(...)`, used at `api.go:870`), not
  by assigning exported fields in a handler. Extend that to roles/streams
  rather than the current `existing.Content = *req.Content` field-poking.
  *(Go deviation: domain aggregates keep exported fields for GORM/JSON
  marshalling; we add `With*` methods and treat direct field assignment
  outside the domain package as a smell.)*
- **Fewer than ~5 public methods per class; ≤4 collaborators per class.**
  Per-aggregate split keeps each service small: `Streams` =
  Create/Update/Delete; `Roles` = Create/Update; `Subscriptions` =
  Subscribe/Unsubscribe/Invite. If one grows past five, split the
  aggregate, don't bloat the type.
- **Small interfaces; depend on capability, not the god-object.** Services
  take the narrow repository interfaces they use (`store.Roles`,
  `store.Streams`, …), never `*store.Store`. The host-side credential
  port is a small noun interface — `APIKeys` with `Service(ctx, orgID)`
  and `User(ctx, userID)` — not an `IdentityProvisioner`.
- **Constructor injection; group accumulated params into an options
  object.** One primary `New(...)`; secondaries delegate. This is acute
  in the composition root: `buildHelixOrgSpawnerConfig` takes ~13
  positional params and `lazyHelixOrgSpawner` ~13 (`helix_org.go:831,
  942`). Group related ones into a `SpawnerDeps`/`RuntimeDeps` options
  struct. *(Go deviation: options structs use exported fields, populated
  at the call site, rather than a fluent builder.)*
- **No globals.** Kill the package-level mutable
  `var helixOrgWorkspaceRef *runtimehelix.Workspace` (`helix_org.go:754`)
  during the composition refactor — it becomes a field on the `Module`
  struct, injected where needed. No new `var`/exported-const surface to
  replace it.
- **Fakes over mocks.** Service tests run against the existing in-memory
  store fake (`infrastructure/persistence/memory/memorystore.go`), not
  gomock. *(Deviation note: the repo-wide `CLAUDE.md` rule "gomock not
  testify/mock" governs the host `helixstore`, which has a generated
  mock; the org module already ships a hand-written in-memory fake and
  that is the right tool for org-application tests — it exercises real
  repository behaviour, not stubbed call expectations.)*
- **No boolean behaviour switches.** A method param may carry an
  orthogonal modifier (a filter, a limit) but must not flip between two
  fundamentally different behaviours — split the method instead.

### 5.1 Introduce a typed application-service layer for structural use cases

Add cohesive services under `application/`, named as the noun they are
(§5.0), that own the structural-mutation use cases with **typed Go
signatures** (no `json.RawMessage`, no synthetic `Caller`). One package
per aggregate, each exposing a single type with ≤5 public methods:

- `application/roles` → type `Roles`: `Create`, `Update`
- `application/workers` → type `Workers`: `Hire`, `UpdateIdentity`,
  `UpdateRole` (`Hire`'s core moves here out of `tools.HireWorker`)
- `application/streams` → type `Streams`: `Create`, `Update`, `Delete`
- `application/subscriptions` → type `Subscriptions`: `Subscribe`,
  `Unsubscribe`, `Invite`
- `application/publishing` → type `Publishing`: `Publish` (store append +
  `Hub.Notify` + `Dispatcher.Dispatch`, the trio that must stay atomic)

Each type takes constructor-injected, narrow collaborators — the specific
repository interfaces it touches (`store.Roles`, `store.Streams`, …) plus
`Topology`/`Hub`/`Dispatcher`/clock/ID-gen as needed — **never the whole
`*store.Store`** (§5.0: ≤4 collaborators, small interfaces). When the
collaborator count climbs, group them in a `Deps`/options struct passed
to one primary `New`. These sit alongside the existing `Dispatcher`/
`Lifecycle`/`Topology` rather than replacing them; `Lifecycle` is the
precedent. Mutations build new aggregate state via `With*` builders, not
field assignment (§5.0, tell-don't-ask).

This respects the helix-org philosophy in `CLAUDE.md`: these services
codify **structural derivation** (the allowed exception — "the Role *is*
the capability"), not workflow. No service may chain multi-step agent
behaviour or subscribe Workers on someone's behalf; that stays
prompt-driven. The litmus test from the philosophy still applies: *"is
the code making a decision the agent should be making?"* — if yes, it
doesn't belong in the service.

### 5.2 Make both adapters thin

- **REST handler** (`interfaces/server/api`): parse → call service →
  map result/error to HTTP. No `orgchart.NewRole`, no `Store.*` calls.
- **MCP tool** (`application/tools`): the tool becomes a protocol
  adapter — unmarshal `Args`, pull `orgID` off `Caller`, call the same
  service, marshal the result. The tool keeps owning the JSON-schema /
  description / MCP-name concern (presentation for the LLM); it stops
  owning the mutation.

Net effect: one use case, one implementation, two thin adapters — the
`hire_worker` pattern, applied uniformly. Drop `Store` from
`api.Deps` and from `server.Server` once the handlers no longer touch it
(compiler enforces the boundary thereafter).

```
            ┌────────────────────────┐     ┌────────────────────────┐
 HTTP/JSON ─▶ interfaces/server/api  │     │ interfaces/server (MCP)│◀─ Worker LLM
            └───────────┬────────────┘     └───────────┬────────────┘
                        │  typed call                   │  typed call
                        ▼                               ▼
                  ┌───────────────────────────────────────────┐
                  │  application services (roles/workers/...)   │
                  │  + existing dispatch / lifecycle / topology │
                  └───────────────────┬─────────────────────────┘
                                      ▼  repository interfaces
                              domain/store (Roles, Workers, …)
```

### 5.3 Extract GitHub integration + credential provisioning out of the root

- Move the inline installation-status and repo-aggregation closures
  (`helix_org.go:403-532`) into the existing `helix_org_github.go` as a
  `gitHubIntegration` adapter type (it's the natural home — the OAuth /
  identity resolvers already live there). `initHelixOrgHandler` then
  just constructs it and passes method values into `apiDeps`.
- Move `ensureHelixOrgServiceAPIKey` / `resolveUserHelixAPIKey`
  (`helix_org_chat.go:88-191`) behind a small noun port — interface
  `APIKeys` with `Service(ctx, orgID)` and `User(ctx, userID)` (not an
  `IdentityProvisioner`; §5.0 no `-er` for the concept, keep the
  interface small). The org module declares the interface; the `server`
  package implements it against `helixStore`. The org module stops
  depending on the host store's user/api-key shape. (This mirrors how
  `ProjectService`/`SpawnerClient` are already inverted via
  `helix_org_inproc.go`.)

### 5.4 Decompose the composition root into focused builders

`initHelixOrgHandler` should read like an assembly manifest. Split the
body into named builders that each return one cohesive thing, e.g.
`openOrgStore` (exists), `buildConfigRegistry`, `buildRuntime`
(in-proc client + spawner + mirror + dispatcher), `buildGitHub`
(integration + resolvers + manifest), `buildAPIDeps`,
`buildPublicWebhooks`. A small **`Module` struct holds the assembled
services**; the top-level function is ~80 lines of "construct X, hand it
to Y." No behaviour moves into new code — it just stops living in one
1020-line function.

Two §5.0 conventions land here specifically:

- **Kill the global.** `var helixOrgWorkspaceRef *runtimehelix.Workspace`
  (`helix_org.go:754`) is the module's only package-level mutable global;
  it exists solely because `buildHelixOrgProjectApplier` had no handle to
  the constructed `Workspace`. It becomes a `Module` field, injected into
  the applier — no global, no hidden initialisation order.
- **Options structs for the wide constructors.** `buildHelixOrgSpawnerConfig`
  (~13 params, `helix_org.go:831`) and `lazyHelixOrgSpawner` (~13 params,
  `helix_org.go:942`) get a single `SpawnerDeps` (or `RuntimeDeps`)
  options struct grouping the related collaborators (store, config
  registry, pubsub, mirror, clock/ID-gen, secret injectors), so the call
  site reads as named fields rather than a positional wall.

## 6. Concern → home mapping

| Concern | Today | Target |
|---|---|---|
| Role/Worker/Stream/Subscription mutations | duplicated in REST handlers + tools | one typed service per aggregate; both adapters delegate |
| Publish (append + notify + dispatch) | inline in `tools.Publish` and `api.publishToStream` | `application/publishing` service |
| Reporting-line edits + reconcile | inline in `addWorkerParent`/`removeWorkerParent` | `workers`/topology service method |
| GitHub install status / repo listing | inline closures in `helix_org.go` | `gitHubIntegration` adapter in `helix_org_github.go` |
| Service/user API-key provisioning | `helix_org_chat.go` reaching into host store | `APIKeys` port, impl in `server` |
| Composition | 1020-line `initHelixOrgHandler` | focused builders + `Module` struct |
| Read/list projections | direct `Store.*` in handlers | optional thin query service (low priority) |

## 7. Migration plan — TDD, incremental, each step green

Every step is **test-first**: write the failing service test against the
in-memory fake store (`infrastructure/persistence/memory/memorystore.go`)
*before* the implementation, then make it pass, then re-point the
adapters. A use case isn't "migrated" until its service has a test that
asserts the invariant the old duplicated paths only implied (e.g.
"create_role via REST and via MCP produce byte-identical store state").

### 7.1 Test gaps to close as we go

These are the use cases we're about to touch that ship **with no test
today** — each must get a fakes-based service test as part of its
migration step (not deferred):

| Area | Untested source |
|---|---|
| Subscriptions | `tools/subscribe.go`, `tools/unsubscribe.go`, `tools/invite_workers.go`, `tools/stream_members.go` (no `_test.go`) |
| Worker mutations | `tools/update_identity.go`, `tools/update_role.go` (no `_test.go`) |
| Reads (for the optional query service) | `tools/read_roles.go`, `tools/read_workers.go`, `tools/read_streams.go`, `tools/read_events.go`, `tools/worker_log.go`, `tools/get_worker_project.go` |
| REST adapter | `interfaces/server/api` has only `api_test.go`, `roles_test.go`, `activate_worker_test.go` — stream/subscription/identity/publish handlers are untested |

`create_role`, `create_stream`, `publish`, `hire_worker`, `dm`,
`reports`, `managers`, `configure_worker_project` already have tool tests
— reuse those as the regression net while the logic moves into services.

### 7.2 Steps

1. **Template: `streams.Streams` service, test-first.** Write
   `application/streams/streams_test.go` (fake store) for
   `Create/Update/Delete` *first*. Implement. Re-point `tools.CreateStream`
   and the REST stream handlers at it; add the cross-adapter parity test.
   Chosen first because there's no existing `update_stream`/`delete_stream`
   tool to untangle. This step also defines the house style every later
   service copies.
2. **`roles.Roles` + `workers.Workers`.** Test-first for `Create`/`Update`
   and `Hire`/`UpdateIdentity`/`UpdateRole`; collapse the duplicated
   tool/REST pairs onto them. Backfill `update_identity`/`update_role`
   coverage here (§7.1).
3. **`subscriptions.Subscriptions` + `publishing.Publishing`.** Test-first
   for `Subscribe`/`Unsubscribe`/`Invite` and `Publish` (assert the
   append→notify→dispatch trio fires in order, via fakes). Fold the
   reporting-line reconcile onto the worker/topology path. Backfill the
   subscription tool tests (§7.1).
4. **Remove `Store` from `api.Deps` and `server.Server`.** Compiler now
   guarantees no interface adapter touches a repository. Update the stale
   `server.go:1-5` package doc that claims "exactly one endpoint".
5. **Extract GitHub integration** from `helix_org.go` into a
   `gitHubIntegration` type in `helix_org_github.go`, with a unit test for
   the install-status sync logic (currently untestable as an inline
   closure).
6. **Invert API-key provisioning** behind the `APIKeys` port (§5.3); test
   the org-module consumer against a fake `APIKeys`, the `server` impl
   against the host store fixtures.
7. **Split `initHelixOrgHandler`** into builders + `Module`; kill
   `helixOrgWorkspaceRef`; group the spawner constructors' params into
   options structs (§5.4).

Steps 1-3 are the high-value DDD work; 4 is the enforcement gate; 5-7 are
the composition-root cleanup and can interleave with 1-4.

Beyond the unit layer, every step is also exercisable end-to-end in the
inner Helix (hire/role/stream flows via the chat UI and the chart) and
against the existing suites (`tools/*_test.go`,
`interfaces/server/api/*_test.go`, `lifecycle_test.go`,
`topology_test.go`).

## 8. Non-goals / guardrails

- **Don't wrap reads dogmatically.** A query service for list/overview
  is optional polish, not a blocker.
- **Don't grow the MCP surface.** Per `CLAUDE.md`, MCP tools stay
  reserved for org-graph primitives. Extracting their logic into
  services does not add tools.
- **Don't move workflow into services.** Services do structural
  derivation only. Orchestration of agent behaviour stays in
  Role/Position prompts.
- **Don't introduce a single god "OrgService" facade.** Per-aggregate
  services keep dependencies honest and match the existing
  `Dispatcher`/`Lifecycle`/`Topology` granularity.

## 9. Open questions — deferred (future)

Parked for now; not part of the initial implementation. Captured so the
decisions aren't lost, to revisit once steps 1-3 have landed and we can
judge from real code.

1. **Tools vs. services ownership of `orgID`/caller.** Services take
   `orgID string` (and an actor id where authz matters) as typed params;
   the MCP tool maps `Caller.OrganizationID()` → that param. Whether to
   thread an actor identity for future audit, or keep "the tool in your
   Role *is* the authz" model unchanged. (Leaning: keep the model; add
   actor id only when we add audit.)
2. **Cheaper stopgap?** Routing the duplicated REST mutation handlers
   through the existing tools (the `hireWorker` synthetic-`Invocation`
   trick) would kill drift faster but propagate the synthetic-`Caller`
   smell. (Leaning: skip it — go straight to typed services.)
3. **`store.Store` god-object.** Whether to split `store.Store` field
   access into per-service interface subsets, or leave the aggregate.
   (Leaning: narrow interfaces for new services; don't churn existing
   ones.)

## 10. Implementation checklist

Ordered, atomic, test-first. Each box is one commit-sized unit. A task
isn't done until `go build ./...` and the relevant suite are green. Land
each PHASE behind its own PR; phases are sequential but tasks within the
"cleanup" phases (E-G) can interleave once the seam (A-D) exists.

> **Status (2026-06-11):** Phases A–C complete — the entire write seam.
> Landed on branch `refactor/helix-org-app-services` (PR
> https://github.com/helixml/helix/pull/2585). Every service has
> fakes-based tests + a REST↔MCP parity test, and each phase was verified
> end-to-end against the live API. Phases D–G (remove `Store` from
> `api.Deps`, GitHub-integration extraction, APIKeys port, composition-
> root decomposition) remain as sequential follow-up PRs.

### Phase A — `streams.Streams` (the template) ✅ defines the house style

- [x] A1. Write `application/streams/streams_test.go` against the
  `memorystore` fake — failing tests for `Create`, `Update`, `Delete`
  (happy path + not-found + org-scoping). No implementation yet.
- [x] A2. Add `application/streams/streams.go`: type `Streams`, one
  primary `New(deps)` taking only `store.Streams` + clock/ID-gen
  (narrow, ≤4 collaborators). Make A1 pass.
- [x] A3. Re-point `tools.CreateStream.Invoke` to call `Streams.Create`
  (tool becomes a thin MCP adapter: unmarshal args → call → marshal).
- [x] A4. Re-point REST `createStream`/`updateStream`/`deleteStream`
  (`api.go:1305-1590`) to call `Streams`; delete the inline
  `Store.Streams.*` calls.
- [x] A5. Add the cross-adapter parity test: MCP `create_stream` and REST
  `POST /streams` produce identical store state.
- [x] A6. `go build ./... && go test ./pkg/org/...`; verify a stream
  create/edit/delete end-to-end (live API: 201/200/404/204).

### Phase B — `roles.Roles` + `workers.Workers`

- [x] B1. `application/roles/roles_test.go` (fake) for `Create`,
  `Update` — assert `With*`-style immutable update, not field-poke.
- [x] B2. `application/roles/roles.go`: type `Roles`, `New(deps)`. Pass.
- [x] B3. Re-point `tools.CreateRole`/`tools.UpdateRole` and REST
  `createRole`/`updateRole` (`roles.go`) at `Roles`; delete the
  duplicated `orgchart.NewRole` + `Store.Roles.*` from both adapters.
  **Fixed a real drift bug**: MCP `update_role` was wiping Tools/Streams.
- [x] B4. `application/workers/workers_test.go` (fake) for
  `UpdateIdentity`, `UpdateRole` (+ `AddParent`/`RemoveParent` in C) —
  **new coverage** (no test today). 
- [x] B5. `application/workers/workers.go`: type `Workers`, `New(deps)`.
  `Hire` deliberately deferred to Phase G (already single-impl; moving
  its ~9 collaborators churns the composition root). Documented in-pkg.
- [x] B6. Re-point the identity/role tools + REST handlers at `Workers`.
- [x] B7. Parity tests (REST vs MCP) for role create and worker identity.
  Build + live-API smoke (role tools/streams preserved on edit).

### Phase C — `subscriptions.Subscriptions` + `publishing.Publishing`

- [x] C1. `application/subscriptions/subscriptions_test.go` (fake) for
  `Subscribe` (idempotent), `Unsubscribe`, `Invite` — **new coverage**.
- [x] C2. `application/subscriptions/subscriptions.go`: type
  `Subscriptions`, `New(deps)`. Pass.
- [x] C3. `application/publishing/publishing_test.go` (fake) asserting the
  `Events.Append → Hub.Notify → Dispatcher.Dispatch` trio fires **in
  order** (fake hub/dispatcher record calls).
- [x] C4. `application/publishing/publishing.go`: type `Publishing`,
  `New(deps)`. Pass.
- [x] C5. Re-point `tools.Subscribe/Unsubscribe/InviteWorkers/Publish`
  and REST `subscribeWorker`/`unsubscribeWorker`/`publishToStream` at the
  two services.
- [x] C6. Fold the reporting-line reconcile in `addWorkerParent`/
  `removeWorkerParent` onto `Workers.AddParent`/`RemoveParent` (cycle
  guard + reconcile); handlers stop calling `Store.ReportingLines.*`.
- [x] C7. Parity + ordering tests; live-API publish/subscribe smoke.

### Phase D — enforce the seam (the gate) ✅

- [x] D1. Removed `Store *store.Store` from `interfaces/server/api.Deps`;
  it now holds the constructed services + a `queries.Queries` read facade
  + `Activations` + a `WorkerRuntime` port + a `GitHubInbound` factory.
  All reads route through `Queries`; the activation pre-create through
  `activations`; the github inbound transport is built at the composition
  root and injected. Builder helpers deleted (construction moved out).
- [x] D2. Removed the `store` field from `server.Server`; it now holds
  `queries` + `publishing`. `mcp.go` resolves worker/role via `queries`;
  `webhook.go` reads the stream via `queries` and appends via
  `publishing`. `NewFromStore` is the composition-time convenience that
  builds those from a store (the Server never holds the store).
- [x] D3. Rewrote the `server.go` package doc to describe the MCP +
  webhook surface routing through services.
- [x] D4. Grep-clean: no `interfaces/server` adapter references a store
  repository. Build + full `./pkg/org/...` suite green.

### Phase E — GitHub integration out of the root ✅

- [x] E1. Added `gitHubIntegration` type in `helix_org_github.go` with
  methods `InstallationStatus` + `AppRepos` (moved verbatim from the
  inline closures). GitHub-call seams (decrypt / listInstalls / mintToken
  / installRepos) are struct fields so the logic is testable.
  (`ManifestStart` already lived in `newGitHubManifestStart`.)
- [x] E2. Added `helix_org_github_integration_test.go` covering the
  install-status sync: install-id + owner backfill, stale-connection
  delete, no-mutation-when-in-sync — against a hand-written fake.
- [x] E3. `initHelixOrgHandler` constructs `gitHubIntegration` and passes
  `InstallationStatus`/`AppRepos` method values into `apiDeps`; the ~130
  lines of closures left `helix_org.go`.

### Phase F — invert API-key provisioning

- [x] F1. Declared `APIKeys { Service(ctx, orgID); User(ctx, userID) }`.
  **Deviation:** declared in the `server` package, not the org module —
  both consumers (the bootstrap middleware and the runtime's
  `BearerForUser` func) are host-side, and the org runtime already
  inverts via its `BearerForUser` func port, so there is no org-module
  code that should depend on an `APIKeys` abstraction. The win the design
  asked for — credential provisioning *out of the composition file,
  behind a small noun port* — is achieved.
- [x] F2. Implemented as `helixAPIKeys` in `helix_org_apikeys.go` (bodies
  of `ensureHelixOrgServiceAPIKey`/`resolveUserHelixAPIKey` moved in);
  deleted from the `helix_org_chat.go` composition file.
- [x] F3. Middleware + spawner `BearerForUser` route through it. Tested
  the impl against the gomock host store + a real config registry
  (User existing/mint, Service mint+flag-grant+cache, no-admin error).

### Phase G — decompose the composition root ✅ (substantive items; full builder split scoped)

- [x] G1. Introduced the `orgServices` bundle (Roles/Streams/Workers/
  Subscriptions/Publishing/Queries/Activations), assembled by
  `buildOrgServices` — the "struct holds the assembled services" shape.
  `initHelixOrgHandler`'s apiDeps literal now lists pre-built services.
- [x] G2. Killed `var helixOrgWorkspaceRef`. The Workspace is a local in
  `initHelixOrgHandler`, injected into `dynamicProjectApplier` as a field
  and threaded through `buildHelixOrgProjectApplier`.
- [x] G3. Grouped `buildHelixOrgSpawnerConfig` + `lazyHelixOrgSpawner`'s
  ~12 positional params into a single `spawnerDeps` options struct.
- [x] G (Hire). Relocated `hire_worker`'s core into `workers.Workers.Hire`
  (deferred from B5). Both adapters delegate; the REST synthetic
  `Invocation` is gone.
- [~] G4. **Consciously scoped:** the further split into
  `buildConfigRegistry`/`buildRuntime`/`buildGitHub`/`buildPublicWebhooks`
  (to reach an ~80-line manifest) was NOT done. Rationale: every piece of
  *business logic* has already been extracted from the root (GitHub →
  Phase E, API-keys → Phase F, Hire → above, the global → G2, the spawner
  param walls → G3, the services → G1), so what remains in
  `initHelixOrgHandler` is genuine wiring. Splitting pure assembly into
  more builders is readability-only churn on critical boot code that
  cannot be end-to-end verified in this dev environment (needs the full
  spec-task sandbox), so the risk/value tradeoff said stop. Each remaining
  builder is a safe, mechanical follow-up.
- [~] G5. Unit + build verification done (32 org packages green, full
  build). Full inner-Helix hire→edit→publish→fire regression needs the
  spawner/sandbox stack not available in this dev environment; Phases A–C
  were verified live, D–G are verified by the test suite + build.

### Done when

- [x] No `interfaces/server` adapter references a `store` repository
  (grep clean).
- [ ] Every migrated use case has one implementation + a fakes-based test
  + a REST↔MCP parity test.
- [ ] `helix_org.go` holds composition only; no business-logic closures,
  no package globals.
