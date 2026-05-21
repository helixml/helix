# 09 — Integration Reframe

**The earlier eight docs (01–08) framed this as "redesign standalone
helix-org." That framing is wrong.** The real direction is the
opposite: helix-org is *being absorbed into helix core*. PR #2286
("first alpha — embedded in Helix behind `HELIX_ORG_ENABLED`") landed
the embedding; PR #2467 consolidated `helix-org/` into the helix
module so both share one Go workspace. The CLI binary still exists but
is a dev-only relic. Production is `helix api` mounting helix-org's
handlers in-process.

This document re-scopes the redesign accordingly. It does **not** rewrite
01–08 — most of the DDD work (contexts, tactical patterns, layering,
SOLID violations) is still correct *within* helix-org. What changes is
the **boundary with the parent helix product** and the **migration end
state**: instead of "a clean standalone Go service", the end state is
"helix-org has been digested by helix and the seam between them is
gone." Several Migrations in `08` shrink, change shape, or evaporate.

---

## 1. Actual integration state (as of `main` at `ee0ecf976`)

Pulled from `api/pkg/server/helix_org.go` (533 LOC of integration
glue), `api/pkg/server/server.go:747-790`, and PR #2286's body.

| Aspect | Reality |
|---|---|
| Build | One Go module; `helix-org/` is a sub-tree of the helix repo. `api` imports `helix-org/...` packages directly (see `api/pkg/server/helix_org.go:17-31`). |
| Deployment | Single `helix api` binary. helix-org's HTTP surfaces (`/ui/`, `/api/v1/org/`, `/api/v1/mcp/helix-org/...`) are mounted by `apiServer.Cfg.HelixOrgEnabled` at `api/pkg/server/server.go:760`. **No standalone helix-org process in production.** |
| Feature gate | `HELIX_ORG_ENABLED` env (deployment-wide) + `alpha_features:['helix-org']` per-user (via the `requireFeature` middleware at `api/pkg/server/helix_org.go:51` and `:768`). |
| Auth | Helix's existing `requireUser + requireFeature` middleware gates every helix-org surface. The "treat every caller as owner" model from `helix-org/CLAUDE.md:33` is dead. Hiring-user identity is forwarded into helixclient via `withHelixUserBearer` at `helix_org.go:266`. |
| Storage | helix-org still owns its own SQLite file at `$FILESTORE_LOCALFS_PATH/helix-org/helix-org.db` (`helix_org.go:85`). **Requires `FILESTORE_TYPE=fs`** — gcs/s3 deployments silently skip the whole feature (`helix_org.go:70-75`). Parallel to helix's own Postgres-backed `store`. |
| Tenancy | **One shared owner Worker across all gated users.** PR #2286 body, OOS list: "Multi-tenant org isolation — every gated user currently shares one owner Worker." `helix_org.go:64-66`. |
| Runtime | Production = `agent/helix` only. `zed_external` + Claude Code OAuth subscription, per-Worker Helix project + agent app + Zed sandbox. The `agent/claude` CLI runtime is dev-only. `agent/helix` calls back to the **same helix process it's running inside** via `helixclient` (loopback HTTP). |
| MCP gateway | Helix already had an MCP gateway. helix-org registers its per-Worker MCP server as a backend: `apiServer.mcpGateway.RegisterBackend("helix-org", NewHelixOrgMCPBackend(...))` at `server.go:784`. helix-org's standalone MCP server (`helix-org/server/mcp.go`) is now an in-process handler invoked by the helix gateway, not a top-level HTTP route. |
| Service API key | Auto-provisioned at boot against the first admin user (`ensureHelixOrgServiceAPIKey`, `helix_org.go:130`); used by the spawner outside request context. |
| Standalone CLI | `cmd/helix-org/{serve,chat,bootstrap,config}` still builds and runs, but is **not the production entry point**. It's now a dev affordance — useful for local demos, end-to-end testing without the rest of helix, and for the embedded demos directory. |

The shape that matters for the redesign:

```
                          ┌─────────────────────────────────────┐
                          │            helix api (one process)  │
                          │                                     │
   /api/v1/*             ─┤ helix routes + auth + orgs/users    │
   /ui/* (alpha-gated)   ─┤ helix-org UI handler (mounted)      │
   /api/v1/org/*         ─┤ helix-org API handler (mounted)     │
   /api/v1/mcp/helix-org │ helix MCP gateway → in-process       │
       /workers/{id}/mcp │   helix-org MCP server               │
                          │                                     │
                          │ ┌─────────────────────────────────┐ │
                          │ │   helix-org module              │ │
                          │ │  (org graph + Streams +         │ │
                          │ │   Activation + Spawner)         │ │
                          │ │                                 │ │
                          │ │   helixclient ◄──HTTP loopback──┼─┼─► same helix
                          │ │   ↑ uses service api_key, then  │ │   process
                          │ │     per-request hiring user     │ │
                          │ └─────────────────────────────────┘ │
                          │                                     │
                          │   SQLite file ($FILESTORE_LOCALFS_   │
                          │   PATH/helix-org/helix-org.db)      │
                          │                                     │
                          │   helix store ──► Postgres          │
                          └─────────────────────────────────────┘
```

Two stores. One LLM-runtime that loops back through HTTP to its own
process. One MCP gateway with two backends. Two tools registries
(helix's `api/pkg/tools/` and helix-org's `tools/`). Two config systems
(helix's `api/pkg/config/` and helix-org's `config/`). Two pubsubs
(helix's `api/pkg/pubsub/` and helix-org's `broadcast/`). **The
absorption is structurally incomplete** — the embedding works, but
helix-org is still a foreign body.

---

## 2. Goal: dissolution, not refactor

The redesign target is not "clean standalone helix-org." It is "helix-org
ceases to exist as a separate concern." The Org Graph, Communication,
Activation, and Transport contexts from `04` survive as helix subsystems;
the supporting infrastructure (helix-org's SQLite, its own pubsub, its
own config, its own MCP gateway, its own helixclient, its own CLI) all
go away.

Concretely the end state is:

- helix-org's domain types (`Worker`, `Position`, `Role`, `Grant`,
  `Stream`, `Event`, `Subscription`, `Environment`, `Message`) live
  under `api/pkg/orgs/` (or similar) in helix.
- Persistence is Postgres via helix's `store.Store`. The SQLite file
  is gone; the `FILESTORE_TYPE=fs` requirement (`helix_org.go:70-75`)
  is gone.
- Pubsub is helix's `pubsub.PubSub`. The `broadcast/` package is gone.
- Config is helix's `api/pkg/config/`. The helix-org `config/` package
  is gone.
- The MCP gateway integration becomes the only MCP surface; there is no
  parallel `helix-org/server/mcp.go`.
- `helixclient.Client` is **deleted**, replaced by in-process function
  calls to helix's existing controllers (Projects, Sessions, Repos,
  Apps, Models, Providers). The 1308 LOC of `helix/helixclient/client.go`
  becomes ~0 LOC.
- The Runtime context survives but its only production impl is "use
  helix's own Projects/Sessions/external-agent infrastructure" — no
  HTTP roundtrips.
- The standalone CLI (`cmd/helix-org/`) stays as a dev affordance — but
  it points at the same helix package by importing `api/pkg/orgs/`,
  not its own copy. Or: it gets deleted entirely and the dev workflow
  moves to "run `helix api` locally."
- Multi-tenancy is wired through helix's existing `Organization` and
  `User` model — an Org Graph belongs to a helix.Organization;
  helix-org Workers are owned by helix.Users.

The migration plan from `08` is **not wrong** but it was written for
the wrong end state. Below I re-orient it.

---

## 3. What changes in the earlier docs

For each prior doc, what survives, what changes, what's obsolete.

### 01 — Inventory

**Survives.** The internal package map, the static dependency graph,
the size+churn map, the load-bearing-file list — all still correct
as a description of the helix-org module today.

**Changes.** Several "entry points" in `01 §1` are no longer the
production entry points:

- The CLI subcommands `serve / chat / bootstrap / config` are dev
  affordances now (`§1.1`). Production entry is `helix api`.
- The HTTP routes table `§1.2` is wrong in production: the routes are
  mounted by helix, not by `helix-org/server/server.go`. The actual
  HTTP route table in production lives in `api/pkg/server/server.go`
  + the mux composition in `helix_org.go:248-256`.
- "Three different ways claude is run" (`§3 #1-3`) is now four, if you
  count helix's own `external-agent` infrastructure for `zed_external`.
- "External integrations" `§2`: helixclient is **not** a third-party
  integration — it's an in-process loopback over HTTP. This is the
  most absurd structural detail of the embedding.

**Obsolete.** Nothing entirely; just re-read with the embedding in mind.

### 02 — Behavioural mapping

**Survives.** All six capability traces are still accurate for the
*helix-org module's internals*.

**Changes.** Each capability has an outer slice that now runs in helix:

- **Cap. 1 (Bootstrap)**: triggered by `initHelixOrgHandler` at API
  startup (`helix_org.go:99`), not by a CLI. Still calls `bootstrap.Run`;
  still seeds one shared `w-owner`. Multi-tenant follow-up is OOS.
- **Cap. 2 (Hire)**: the chat brain runs as the logged-in helix user
  (`withHelixUserBearer` at `helix_org.go:266`), not as an anonymous
  owner. `tools/hire_worker.go`'s direct `agenthelix` import is still
  the same DIP violation `04 §4 cut #1` flagged, *and* now the
  ProjectApplier it calls makes loopback HTTP to the same process.
- **Cap. 3 (Inbound event)**: same shape; webhook receivers are still
  helix-org's own routes mounted under `/api/v1/org/` and the
  transport-specific paths.
- **Cap. 4 (Worker reads org graph)**: same in-process; just enters
  through helix's MCP gateway (`/api/v1/mcp/helix-org/workers/{id}/mcp`)
  instead of the standalone endpoint.
- **Cap. 5 (Webhook demo)**: still works, but the curl now hits the
  helix host, not a standalone helix-org port.
- **Cap. 6 (helix-org chat CLI)**: dev-only. The production owner-chat
  is `/ui/` mounted in helix.

**Obsolete.** The "operator types in a terminal and `syscall.Exec` runs
claude" story is dev-only now.

### 03 — Ubiquitous language

**Survives.** Domain glossary, homonym list, synonym list — all valid.

**Changes.** Two new homonyms emerge at the helix↔helix-org seam:

- **"Project"**: helix has `Project` (a top-level workspace concept);
  helix-org's Spawner provisions a `helix.Project` per Worker via
  `ProjectApplier`. Inside helix-org, "Project" only means the helix
  thing — but every helix-org Worker now corresponds to a helix
  Project, and that's a 1:1 relationship the schema doesn't make
  explicit.
- **"Agent"**: helix already has its own `AgentApp` / `external-agent`
  concept. `03 §2.1` listed "Helix Agent App" as one of the five
  meanings of "agent." That was correct; what's new is that in the
  integrated codebase, the helix-side and helix-org-side "agent"
  vocabularies sit in the same Go module — the homonym is now intra-
  module.
- **"Organization"**: helix has `Organization` (multi-tenancy unit);
  helix-org has no `Organization` type — the org-graph is implicitly
  global. The word "org" in `helix-org` means **the graph**, not the
  helix Organization. PR #2286 calls this out: "Multi-tenant org
  isolation — every gated user currently shares one owner Worker."

**Adds to the "resolve first" list (`03 §6`)**:

- **9. Resolve `Organization` (helix) vs `Org Graph` (helix-org).**
  Either rename helix-org's concept ("Workspace"? "Roster"?) or scope
  one Org Graph instance per helix.Organization.
- **10. Resolve `Project` 1:1 with `Worker`.** Today it's implicit in
  `ProjectApplier.Ensure(workerID)`. Make it explicit: either
  `Worker.HelixProjectID` is a real field, or stop having a 1:1 and
  share Projects across Workers.

### 04 — Bounded contexts

**Survives.** Seven contexts (`§1`) still hold. Tactical patterns
(`§2`) still hold.

**Changes.**

- **MCP Gateway** (`§2.6`): no longer "the only HTTP surface this
  context cares about" — helix-org's MCP server is now a backend of
  helix's MCP gateway. The context still exists but its host moves.
- **Operator Surface** (`§2.7`): the chat backend + UI handlers still
  belong here, but **operational config** merges into helix's existing
  config system. Two parallel config registries is a transient state.
- **Agent Runtime** (`§2.5`): the `helixclient` infrastructure that
  the helix runtime uses dissolves into direct controller calls in
  helix. The Runtime port survives; the Helix impl becomes "use helix
  controllers."
- **Cross-cuts** (`§4`): cut #1 (hire reaches into runtime), cut #2
  (dispatcher mixes activation + outbound), cut #3 (chat imports
  runtime), cut #4 (transport parsers in domain), cut #5 (serve.go
  switch ladders), cut #7 (three Dispatcher interfaces) — **all still
  valid and still need fixing.** Cut #6 (UI bypasses MCP) is reframed:
  the UI is now helix-authenticated, so "bypass" is fine; the doc just
  needs to say so. Cut #8 (two bootstraps) is reframed: only one
  bootstrap path remains in production (`initHelixOrgHandler`); the
  CLI `bootstrap` subcommand is dev-only.
- **Provisional ownership table** (`§6`): every target package moves
  from `<helix-org>/orgs/` etc. to `api/pkg/orgs/` in helix. The
  packages I named are still right; the parent is wrong.

### 05 — Tactical patterns

**Survives.** All entity / VO / aggregate / domain-event / invariant
proposals still hold. The five priority moves at the end (`§9`) all
still apply.

**Adds.** Two new aggregates surface at the helix seam:

- `WorkerHelixBinding` (or fold into Worker) — `{WorkerID,
  HelixProjectID, HelixAgentAppID, HelixRepoID, HelixSessionID}`.
  Currently lives in `WorkerRuntimeState` as a sidecar kv-store
  (`agent/helix/state.go:65-71`); should be typed and primary.
- `HelixIdentity` value object — the per-call hiring user's
  `(HelixUserID, BearerToken)` forwarded via middleware
  (`helix_org.go:266`). Replaces the "treat all callers as root" model.

### 06 — Layers, ports, primitives

**Survives.** Package classification, ports list, prompt-placement,
primitive-obsession audit — all valid.

**Changes.**

- The "missing port `Runtime`" finding becomes more pointed: with only
  one production runtime (`agent/helix` over helix controllers) and
  one dev runtime (`agent/claude`), the Runtime port is the seam
  between "this is helix" and "this is a local claude shell." That's
  a meaningful binary; design the port for it.
- The "missing port `ChatSession`" finding becomes critical: with one
  production chat surface (helix-org `/ui/`) plus the dev CLI, the
  port unblocks killing `server/chat/helix_bridge.go`'s direct
  `agenthelix` import (`07 DIP` headline).
- helixclient's 1308 LOC ISP split (M8) is reframed: don't split,
  **delete**. Loopback HTTP to the same process is replaced by direct
  controller method calls.

### 07 — SOLID

**Survives.** All concrete file-and-struct findings still valid.

**Changes.**

- M8 in `08` was "split helixclient.Client by concern (Auth, Projects,
  Repos, Sessions)." Reframe: this is an ISP move only as a stepping
  stone toward deletion. The end state is "helixclient does not
  exist." See `09 §4 M-helix-1` below.

### 08 — Migration plan

This is the doc that needs the biggest re-orientation. The validation
in Part A still holds (the proposed shapes do make Caps 1–6
straightforward). But the **migration order** in Part B was sequenced
for a standalone-redesign world. Re-sequenced for the integration
world in `§4` below.

---

## 4. Re-sequenced migration plan

Two parallel tracks. Track A is the dissolution work — peeling helix-org
into helix. Track B is the DDD work inside helix-org/ that should ship
before, during, and after the dissolution moves. They interleave.

**Track A — Dissolution moves** (helix-org → helix)

| # | Title | Effect | Effort |
|---|---|---|---|
| H1 | **Replace helixclient loopback with direct controller calls** | Delete most of `helix/helixclient/client.go` (~1300 LOC). Spawner, ProjectApplier, chat bridge call helix controllers directly. | L |
| H2 | **Replace helix-org's broadcast pubsub with helix's `pubsub.PubSub`** | `broadcast/` package gone; subscribers move to helix pubsub topics. | S |
| H3 | **Replace helix-org's config registry with helix's `api/pkg/config/`** | `helix-org/config/` gone; settings live in helix's config infrastructure. | M |
| H4 | **Move helix-org's data from SQLite → Postgres via helix's `store`** | The `FILESTORE_TYPE=fs` requirement disappears (`helix_org.go:70-75`); the feature works on gcs/s3 deployments. | L |
| H5 | **Multi-tenant the Org Graph: one instance per helix.Organization** | Drops the "shared owner Worker across all gated users" constraint from PR #2286 OOS. | L |
| H6 | **Identity model: helix.User drives hiring, not "w-owner = everyone"** | Replaces `withHelixUserBearer` shim with first-class `HelixIdentity` flowing through Activation. | M |
| H7 | **Standalone CLI becomes dev-only or is deleted** | `cmd/helix-org/` is either marked clearly as dev-affordance (with no production config paths) or removed entirely. | S |
| H8 | **Move helix-org packages from `/helix-org/...` to `api/pkg/orgs/...`** | One Go module reorganisation; nothing semantic. Last move; symbolic completion. | M |

**Track B — Internal DDD moves** (from `08`)

These all still apply. Re-numbered for clarity; the work is the same.

| New # | Old (`08`) # | Title | Notes after reframe |
|---|---|---|---|
| B0 | M0 | ADR-0001 terminology pinned | Add resolutions for the two new homonyms (`Organization`, `Project`) — see `09 §3.03`. |
| B1 | M1 | Transport parsers out of `domain/` | Unchanged. |
| B2 | M2 | Split Dispatcher → Scheduler + Outbox | Unchanged. |
| B3 | M3 | Runtime port + registry | Smaller scope: in production there's one runtime (helix). Port still useful for dev-claude and for future runtimes. |
| B4 | M4 | Decouple hire from Runtime via `WorkerHired` event | **Pairs naturally with H1.** When `tools/hire_worker.go` stops calling `agenthelix.ProjectApplier` directly, that call moves to a `WorkerHired` subscriber. Once H1 lands, that subscriber is direct controller calls, not helixclient. |
| B5 | M5 | Promote `Activation` to a first-class aggregate | Unchanged. |
| B6 | M6 | Typed `Message` VO + `Principal` VO | Unchanged. |
| B7 | M7 | `Role.DefaultTools` + `DefaultStreams` | Unchanged. |
| B8 | M8 | Split `helixclient.Client` by concern | **Mooted by H1.** Don't split; delete on the way to H1. |
| B9 | M9 | Consolidate three `claude` invocation sites | Reframe: only the AI Worker Spawner survives in production. CLI chat is dev. Owner-chat in `/ui/` no longer uses claude exec — it uses the helix `external-agent` infra. Consolidation still useful for dev. |
| B10 | M10 | Owner UI vs MCP Gateway contract | **Resolved by the embedding.** The owner UI is helix-authenticated, server-side, and trusted. Doc fix only — `helix-org/CLAUDE.md:19`. |
| B11 | M11 | Doc sweep | Add helix-org/CLAUDE.md's CLI-centric prose to the list; replace with the "embedded in helix" reality. |

### Recommended order

The cheapest, highest-clarity order interleaves the two tracks:

1. **B0** (terminology + ADR-0001) — pins names that affect every later PR.
2. **B11** (doc sweep) — get `CLAUDE.md` aligned with the embedded reality.
   These are sub-day moves; do them now so the rest of the work has
   accurate orientation.
3. **B1** (Transport parsers out of `domain/`) — small move that paves
   the way for H4's clean schema migration.
4. **B7** (Role.DefaultTools / DefaultStreams) — small win, closes
   TODO.md item 1.
5. **H7** (standalone CLI dev-only) — declarative move; mark the binary
   as dev-only, remove production-config paths from it.
6. **H2** (replace broadcast with helix pubsub) — smallest dissolution
   move; validates the pattern.
7. **B2** (Scheduler + Outbox split) — pairs with H2 (both rewire
   event-flow scaffolding).
8. **B3** (Runtime port + registry) — pre-req for H1; also closes the
   `serve.go` switch-ladder smell.
9. **B4 + H1** (paired) — the headline. `tools/hire_worker.go` stops
   importing `agenthelix`; the `WorkerHired` subscriber moves to
   direct helix controller calls. **helixclient's 1308 LOC drops to
   near-zero.**
10. **B9** (consolidate claude invocations) — naturally lands after H1
    because the chat bridge no longer needs the helixclient adapter.
11. **B5** (Activation aggregate) — done after the runtime port is
    clean.
12. **B6** (typed Message + Principal) — done in parallel with B5; both
    are internal data-shape changes.
13. **H3** (config registry merge) — done once helix-org's
    `config.Registry` callers are stable.
14. **H4** (storage migration to Postgres) — big, but unblocks running
    on gcs/s3 deployments; do it once the schema has stabilised after
    B5/B6.
15. **H5 + H6** (multi-tenant + identity model) — depends on H4 (need
    Postgres for `(org_id, worker_id)` keys) and H1 (need direct
    controller access for clean identity flow).
16. **H8** (move packages into `api/pkg/orgs/`) — symbolic completion.
    Do this last; it's churn-heavy and risks merge pain if done early.

### Two-sprint slice (replacing `08 §F`)

**Sprint 1**
- B0, B11 — terminology + doc sweep (1 day)
- B1 — transport parsers out (2 days)
- B7 — Role.DefaultTools/Streams (1 day)
- H7 — mark CLI dev-only; remove production-config paths from it (1 day)
- Two ADRs (terminology, embedded reality) — 1 day

**Sprint 2**
- H2 — broadcast → helix pubsub (2 days)
- B2 — Scheduler + Outbox split (3 days)
- B3 — Runtime port + registry (3 days)
- ADRs (Runtime port, pubsub unification) — 1 day

End of sprint 2: helix-org's internals are tidier and one piece of
infrastructure (pubsub) has been absorbed into helix. Sprint 3+ is
where the headline pairing **B4 + H1** lands — the single largest
LOC reduction in this whole redesign.

---

## 5. Decisions deferred

Three decisions are *not* made by this plan and need their own ADRs
when the work approaches them.

1. **Does the helix-org `tools` registry merge with helix's
   `api/pkg/tools`?** Helix has its own tools registry (for Apps). The
   two have similar shapes but different concerns. Options:
   - (a) keep them separate, surfaced through one MCP gateway as two
     namespaces. Status quo.
   - (b) merge into one registry. Higher leverage long-term; bigger
     scope.
   Recommendation: defer until both registries are stable post-H1/H4.
   Don't merge speculatively.

2. **Does helix-org's `Worker` merge with helix's user/agent
   abstractions?** Helix has Users, Memberships, AccessGrants;
   helix-org has Workers, Subscriptions, Grants. These overlap in
   shape but the helix-org concepts are richer (Role markdown,
   Identity content, per-Worker Environment). Likely answer: they
   stay separate, but `Worker.HumanCounterpart` becomes a typed
   reference to a `helix.User`. ADR after H5/H6.

3. **Where does the MCP gateway live?** Today it's split: helix has a
   gateway that holds backends, helix-org has its own per-Worker MCP
   server registered as one backend. If multi-tenancy lands (H5), there
   will be N owner Workers, each with M hired Workers — does the
   gateway expose `/api/v1/mcp/helix-org/<org-id>/workers/<worker-id>/mcp`?
   ADR after H5.

---

## 6. Net assessment

The earlier eight docs identified the right structural problems but
mis-named the destination. The dissolution-into-helix trajectory
means:

- **helixclient's 1308 LOC is the single biggest LOC win** — bigger
  than any of the splits proposed in `06` / `07`.
- The **single owner Worker shared across all gated users** is a
  bigger constraint than the docs suggested; multi-tenancy (H5) is on
  the path even though PR #2286 explicitly defers it to a future
  alpha.
- The "operational config" / "owner UI" / "MCP gateway" cleanups in
  earlier docs become smaller because helix already has equivalents
  — the work is *absorption*, not refactor.
- The **DDD work inside helix-org (the seven contexts, the typed
  aggregates, the unenforced invariants) is unchanged** and should
  ship in parallel with the dissolution. The end-state package
  `api/pkg/orgs/` is structurally the same as the standalone redesign
  imagined — it's just sitting inside helix rather than next to it.

The framing error in 01–08 is symptomatic of the documents the prior
agents read: `helix-org/CLAUDE.md` still describes the standalone
project, the dev tooling, and the standalone CLI as if they were
production. PR #2286's body is the authoritative description of where
this is going. Promoting that PR body's contents (or its companion
design doc `design/2026-05-17-helix-org-saas-alpha.md`, which the PR
notes was deleted post-integration but lives in git history) into
`helix-org/CLAUDE.md` is itself a small valuable move — B11 should
include it.
