# 08 — Validation and Incremental Migration

Final step. Two parts:

1. **Validate the proposed model** by re-walking the behavioural traces
   from `02-behavioral-mapping.md` against the target shape — does each
   capability become straightforward without losing necessary flexibility?
2. **Sequence the migration** as a strangler fig: one extracted context
   at a time, smallest credible move first, ADRs written as we go,
   tracked metrics so we can tell whether we're actually getting better.

This document is meant to be the *plan of record* the redesign work
runs against. Each "Migration" below is one PR-shaped slice with a
clear definition of done.

---

## Part A — Validation against the six capabilities

For each capability from `02-behavioral-mapping.md`, walk the proposed
shape (the seven contexts in `04`, the tactical patterns in `05`, the
layering in `06`) and check the awkward bits become straightforward.

### Cap. 1 — Bootstrap an Org

**Today's awkwardness** (`02 Cap. 1`):

- Two different "bootstraps" (`serve` seeds; `bootstrap` is a Helix
  pre-flight).
- Owner activation stream is created but never written to.
- Owner Role embedded via `//go:embed` in `bootstrap/`.
- `bootstrap/` references `agent.PublishActivationEvent` that doesn't
  exist.

**In the target shape.**

- `Org Graph` aggregate exposes `OrgGraph.SeedOwnerIfEmpty()` (`04 §4
  cut #8`, `05 §1`). The CLI `bootstrap` subcommand is renamed to
  what it actually does (`helix-runtime preflight`).
- Owner activation Stream is created **inside the Activation aggregate**
  on first owner activation (lazy), not eagerly at bootstrap — kills
  the perpetually-empty-stream artefact (`02 Cap. 1 pain point 2`).
- `bootstrap/templates/owner_role.md` moves to `org/seed/owner_role.md`
  per `06 §4`. Still embedded; now in the right layer.
- The dead `agent.PublishActivationEvent` reference disappears once
  Activation is a real aggregate emitting `ActivationSegmentAppended`
  (`05 §3 domain events`).

**Verdict.** Cleaner. No new flexibility lost: an operator who wants a
custom owner Role still gets it via post-bootstrap edits, same as today.

### Cap. 2 — Owner hires a Worker

**Today's awkwardness.**

- `tools/hire_worker.go` imports `agent/helix` + `helix/helixclient`
  directly and conditionally calls `WorkerProject.Ensure` (`04 §4
  cut #1`, `06 §3`, `07 DIP`).
- Hire does *not* subscribe the new Worker to Streams; the LLM has to
  remember (`02 Cap. 2 pain point 1`; `TODO.md` item 1).
- Grants must be bundled into the hire call; the social contract is
  enforced by the owner's role markdown (`02 Cap. 2 pain point 2`).
- "Did the hire complete?" has no causal signal — the owner polls
  `worker_log` with a 30s wait (`TODO.md` item 2).

**In the target shape.**

- `hire_worker` becomes a pure `Org Graph` operation. It emits a
  `WorkerHired` domain event (`05 §1 domain events`). Subscribers:
  - `Agent Runtime`'s `OnWorkerHired` ensures the Helix project (was
    `WorkerProject.Ensure`). Lives in `runtime/helix/`. **Org Graph
    no longer imports Runtime.**
  - `Communication`'s `OnWorkerHired` reads `Role.DefaultStreams`
    (newly typed, `05 §8 carve-out`) and creates the Subscriptions
    automatically — closes `TODO.md` item 1.
- Hire returns an `ActivationID` (the hire-Activation) so the owner
  can `read_events(activation_id=…)` instead of `worker_log` with a
  string-match for `=== exit: ok ===` (`05 §3 Activation aggregate`).
  Closes `TODO.md` item 2.
- `Grant` lifecycle is unchanged but Role gains `DefaultTools` — the
  hire path now refuses to hire a Worker without grants if the Role
  declares any (invariant on `Org Graph` aggregate, `05 §6`).

**Verdict.** Strictly more checks at the seam. The "hire workflow is
prose, code, prose, code" pattern of today collapses into one
transactional aggregate op + N event subscribers. The flexibility
sacrificed (you can no longer mix-and-match grants vs Role) is the
flexibility the owner role markdown is currently working around.

### Cap. 3 — Inbound event → activation

**Today's awkwardness.**

- Three trigger paths (`publish`, generic webhook, typed transport)
  with overlapping but not identical Message shaping.
- Dispatcher mixes activation queueing with outbound emit (`04 §4 cut
  #2`).
- Coalescing only fires *while* a Spawner is running, not at cold
  start — TODO.md item 6.
- `Source` is empty for everything inbound; Roles can't reliably tell
  external from in-org (`02 Cap. 3 pain point 3`).

**In the target shape.**

- All three paths funnel through a single `Stream.Append(Message)`
  aggregate op (`05 §2`). The Message envelope is a typed VO, not a
  JSON-blob `Body TEXT` (`06 §5 primitive obsession`).
- `Dispatcher` becomes pure activation scheduling; outbound emit is
  an `Outbox` (`04 §4 cut #2`, `05 §3`, `06 §6`). Both subscribe to
  the `EventAppended` domain event of `Stream`.
- The per-Worker queue's coalescing window opens *as soon as the first
  trigger arrives* (small fixed delay, e.g. 100ms) instead of "only
  if a Spawner is already running" — closes TODO.md item 6 properly
  (`05 §9 fix #5`).
- `Source` becomes a typed `Principal` (`{Kind, ID}` where Kind ∈
  {`worker`, `transport`, `human`}) — replaces both `Event.Source` and
  `Message.From` (`06 §5`).

**Verdict.** This is the highest-leverage cleanup. Cap 3 is where every
production failure will live.

### Cap. 4 — Worker reads the org graph

**Today's awkwardness.**

- `read_events` is union-of-streams; no per-stream filter (`02 Cap. 4
  pain point 1`).
- `since` is positional, not causal.
- MCP server rebuilt per request — cheap today, fragile later.

**In the target shape.**

- `read_events` gains an optional `stream_id` filter — pure Communication
  query method on the Subscription set. Trivial.
- `since` becomes causal: events are keyed by `(StreamID, SequenceInStream)`.
  The activation transcript already orders within an Activation; this
  lifts the same idea to Stream level. Optional — not in the critical
  path.
- The MCP rebuild stays — it's the right design for stateless HTTP.
  Just move the three DB calls in `server/mcp.go:67-114` behind a
  single repository read (`06 §8 punch list`).

**Verdict.** Small wins; no architectural friction.

### Cap. 5 — Webhook demo end-to-end

This is just Caps 1+2+3+4 composed. Once each is straight, the demo
trace gets boring — which is the goal.

The one demo-specific awkwardness — *roles encode workflow in
markdown and silently fail if the LLM skips a step* — is partially
addressed by lifting `DefaultStreams` onto Role (so subscription is
no longer prose-only). The rest stays prose, by design (`05 §8`).

### Cap. 6 — `helix-org chat`

**Today's awkwardness.**

- Docstring lies about `--continue` vs `--resume`.
- `CLAUDE.md` claims a `--install-claude-mcp` path that doesn't exist.
- The "UI and terminal share a conversation" claim only holds if both
  run from the same cwd (`02 Cap. 6 pain point 3`).

**In the target shape.**

- All three claude invocation sites (CLI chat, UI chat, AI Worker
  Spawner) consolidate behind one `ClaudeSession` type in
  `runtime/claude/` (`01 §6 bullet 10`, `06 §2 missing ports`). One
  place to fix mcp.json assembly; one place to define session-resume
  policy.
- The doc/code drift gets resolved in the same PR that consolidates.

**Verdict.** Mostly documentation work. The architectural value is
*future*: a third runtime (or a third invocation site) costs O(1) not
O(3).

### Overall validation

The proposed model makes the awkward things from `02` straightforward
without removing any actually-used flexibility. The two pieces of
flexibility the redesign *does* remove are both implicit social
contracts the owner role markdown is currently working around — that
is the point.

---

## Part B — Strangler-fig migration

### Principles

- **Move one context at a time.** Pick a supporting context first to
  derisk the pattern. Core context refactors come after the seam shape
  has been validated.
- **Behind a port, then move the impl.** Every move starts by defining
  the port at its new home, then forwards the old call sites through
  the port, *then* moves the implementation file. Tests run at each
  step.
- **One ADR per non-obvious decision.** `design/adr/NNNN-<slug>.md`,
  numbered sequentially. ADRs are short (one page).
- **Track metrics from M1 onward.** See §Metrics below. The point isn't
  to celebrate numbers; it's to notice if a refactor *increases* the
  thing it was meant to decrease.
- **Don't bundle moves.** Each Migration below is one PR.

### Migration order (proposed)

The order is chosen to: (a) start with a supporting context, (b) put
each "I import a concrete from a sibling" cut early so subsequent moves
inherit clean seams, (c) keep the highest-leverage core refactor
(Activation aggregate) for last when the surrounding work has
de-risked it.

| # | Title | Context touched | Type | Effort | Unblocks |
|---|---|---|---|---|---|
| **M0** | ADR-0001: terminology pinned (Stream, Identity, Activation, Worker, Tool) | All | Doc | S | Every later PR can use names without back-and-forth |
| **M1** | Lift `Transport` parsers out of `domain/`; introduce `TransportRegistry` | Transports | Supporting | S | M3 (clean shape for adding a transport) |
| **M2** | Split `dispatch.Dispatcher` → `activation.Scheduler` + `transport.Outbox` | Activation + Transports | Cross-cut | M | M5 (Scheduler is the Activation-aggregate scaffold) |
| **M3** | Promote `Runtime` port; collapse `agent/{claude,helix}` selection into a registry; remove `serve.go` switch ladders | Agent Runtime | Supporting | M | M4 |
| **M4** | Remove `tools/hire_worker.go`'s direct `agent/helix` import; introduce `WorkerHired` domain event | Org Graph + Runtime | Cross-cut (DIP) | M | M6 |
| **M5** | Promote `Activation` to a first-class aggregate with `ActivationID`; new `activation` package; transcript is its own log | Activation | Core | L | TODO.md item 6 fix |
| **M6** | Typed `Message` VO on `Event.Body`; typed `Principal` collapses `Event.Source` + `Message.From` | Communication | Core | M | Cap 3 cleanup; M7 |
| **M7** | Lift `Role.DefaultTools` + `Role.DefaultStreams` as typed fields; enforce hire invariants | Org Graph | Core | S | TODO.md item 1 |
| **M8** | Split `helixclient.Client` into sub-interfaces (auth, projects, repos, sessions) | Agent Runtime (ISP) | Hygiene | S | — |
| **M9** | Consolidate three `claude` invocation sites behind one `ClaudeSession` | Agent Runtime (SRP) | Hygiene | M | Cap 6 cleanup |
| **M10** | Resolve owner-UI-vs-MCP-Gateway contract (doc fix OR route owner UI through MCP) | Operator Surface | Policy | S | Closes the `CLAUDE.md` contradiction at `01 §6 #4` |
| **M11** | Doc sweep: `CLAUDE.md` aligned with code (Channel→Stream, scope removed, `--install-claude-mcp` removed, `bootstrap`-renames documented) | Docs | Doc | S | — |

S = sub-week, M = 1–2 weeks, L = 2–4 weeks.

### Migration detail

Below: target slice for each Migration. Each says **what lands**, **how
to verify**, **risk**, **rollback**.

#### M0 — ADR-0001: terminology

Write ADR-0001 fixing the eight terminological decisions from
`03 §6` items 1–8. Edit `CLAUDE.md`'s structural-primitives list
(`CLAUDE.md:19`) to remove "Channels" and to remove "scope". Rename
`agent.md` → `worker-policy.md` (one find/replace plus
`agent/policy.go` rename). No code logic changes.

**Verify**: `grep -ri "channel\|scope" --include="*.go"` returns only
intended hits (e.g. `make/cli/mcp` channel-as-Go-chan).

**Risk**: zero functional. Some downstream demo role markdown uses
"channel" colloquially — leave those; they're prose.

**Rollback**: revert.

#### M1 — Transport parsers out of `domain/`

Move `domain/transport.go`'s parse/validate switch (per `04 §4 cut #4`)
into `transports/<x>/` packages. Introduce a `TransportRegistry` in a
new `transports/registry.go`. `domain/transport.go` keeps only the
discriminator + `Config json.RawMessage`.

**Verify**: `domain/transport.go` shrinks from 315 LOC to ~50 LOC;
adding a new TransportKind no longer requires editing `domain/`.

**Risk**: low. Single-purpose move.

**Rollback**: trivial (one file move).

#### M2 — Split Dispatcher into Scheduler + Outbox

Today `dispatch/dispatcher.go` does both per-Worker activation queueing
and outbound webhook/email emit (`04 §4 cut #2`). Split:

- `activation.Scheduler` (new package `activation/`) — owns the
  per-Worker queue + coalescing. Subscribes to `EventAppended`.
- `transport.Outbox` (new sub-package of `transports/`) — owns the
  emit-to-external-system path. Subscribes to `EventAppended`.
- The `EventAppended` event bus is a tiny in-process pub/sub
  (initially `broadcast.Broadcaster` repurposed; later its own type).

Tools' `Dispatcher` and `server.Dispatcher` interfaces collapse into
one `activation.Scheduler` interface (`03 §6 item 6`, `07 ISP`).

**Verify**: no remaining duplicate `Dispatcher` interface; `dispatch/`
package is gone or only re-exports for compat.

**Risk**: medium. Test coverage for `dispatch/` (711 LOC tests, `01
§5.1`) needs to follow. Mock wiring in tests must be updated.

**Rollback**: keep `dispatch.Dispatcher` as a thin facade over both
new components for one release; revert if needed.

#### M3 — Runtime port + registry

Lift a single `Runtime` interface (`06 §2 missing ports`):

```go
type Runtime interface {
    Spawner
    WorkspaceSync
    Name() string
}
```

Introduce a `runtime.Registry` populated at process start. Move
`buildSpawner` / `buildChatBackend` switch statements (`01 §6 bullet
5`, `serve.go:247-326`, `:343-437`) into per-runtime `init()` funcs
that register themselves. `main` becomes wiring of pre-built objects.

**Verify**: `cmd/helix-org/serve.go` shrinks past 300 LOC; adding a
third runtime no longer requires touching `serve.go`.

**Risk**: medium. The two runtimes have subtle LSP divergences
(`07 LSP`) — surface them in the port docs and add interface tests
that both impls run against.

**Rollback**: keep the old switch as a fallback for one release.

#### M4 — Hire decouples from Runtime via `WorkerHired` event

The headline DIP violation (`04 §4 cut #1`, `06 §3`, `07 DIP`,
`tools/hire_worker.go:12-15`). Steps:

1. Define `org.WorkerHired` domain event.
2. `tools/hire_worker.go` stops importing `agent/helix`. The tool
   inserts rows, then emits `WorkerHired`. **No knowledge of which
   runtime exists.**
3. Move `WorkerProject.Ensure(...)` into `runtime/helix/`'s
   `OnWorkerHired` subscriber.
4. Move "create activation stream + subscribe hiring worker" into the
   Activation context's `OnWorkerHired` subscriber.

**Verify**: `tools/hire_worker.go` no longer imports any `agent/*` or
`helix/*` package; runtime tests pass; Helix-backend hire still
materialises a Project on the parent Helix server.

**Risk**: medium. The current synchronous "create Helix project before
hire returns" becomes asynchronous (the subscriber runs after the row
insert). If the project-creation call fails, the Worker exists in the
DB but not in Helix — needs a reconciler or a saga step. Worth one
ADR.

**Rollback**: re-introduce the direct call as a degenerate
subscriber.

#### M5 — Activation aggregate

The biggest move (`05 §3`, `05 §9 fix #1`). Introduces:

- `activation.Activation` entity: `{ID, WorkerID, Triggers, StartedAt,
  EndedAt, Outcome, TranscriptStreamID}`.
- `activation.Transcript` as the typed log instead of "a Stream with a
  magic ID convention" (`03 §6 item 8`).
- `worker_log` MCP tool gains `activation_id` filter.
- `hire_worker` returns the `ActivationID` of the hire-Activation —
  closes TODO.md item 2.
- Coalescing window opens at first trigger, not "if Spawner already
  running" — closes TODO.md item 6 (`05 §9 fix #5`).

**Verify**: `worker_log` can filter to a single activation; the hire
end-to-end test asserts the returned ActivationID can be queried.

**Risk**: medium-high. This touches everything: Spawner contract,
transcript shape, the owner-chat poll loop. Land behind a feature
flag — `activation.v2 = true|false` config key — and keep both code
paths until the v2 path is exercised by every demo.

**Rollback**: feature flag off.

#### M6 — Typed Message + Principal

Two value-object promotions in one PR (`06 §5`):

- `Event.Body` typed as `Message` (the existing struct), parsed at the
  storage boundary instead of `dispatch/dispatcher.go:137-141`.
- `Principal` value object: `{Kind, ID}` where `Kind ∈ {worker,
  transport, human}`. Replaces both `Event.Source` and `Message.From`
  in the canonical representation; transports translate at the
  boundary.

**Verify**: `dispatch/dispatcher.go`'s parse-fallback at `:137-141` is
gone (the typed Body never fails to parse, because every write goes
through `Stream.Append(Message)`).

**Risk**: medium. Schema-shape change for Events — write a migration
that re-parses the JSON `Body` column once at boot (still
`AutoMigrate` semantics; no SQL migration files per `CLAUDE.md:38`).

**Rollback**: revert; data is forward-compatible.

#### M7 — Role.DefaultTools + Role.DefaultStreams

Lift the two markdown-section conventions (`prompts/templates/role.md`
`## Tools (MCP)` and `## Streams`) into typed fields on the Role
aggregate (`05 §8 carve-out`, `04 §4 cut #6` validation). Hire reads
these and refuses to hire without grants if the Role declares any.

**Verify**: closes TODO.md item 1. Demo roles continue to work
because the new fields default empty if the markdown sections are
absent.

**Risk**: low. Backwards-compatible additions.

**Rollback**: ignore the new fields.

#### M8 — Split `helixclient.Client`

`helix/helixclient/client.go` is 1308 LOC and 13 method-groups (`01
§5.2`, `07 ISP`). Split the `Client` interface into:

- `helixclient.Auth` (whoami)
- `helixclient.Projects`
- `helixclient.Repos`
- `helixclient.Sessions`
- `helixclient.Server` (server status, models)

The struct stays one impl; the interfaces let callers (`tools/`,
`server/chat/`, `agent/helix/`) take only what they need.

**Verify**: callers no longer have `Client.AllTheMethods` in their
mock surface.

**Risk**: low. Pure ISP move.

**Rollback**: trivial.

#### M9 — Consolidate the three `claude` invocation sites

`agent/claude/spawner.go` + `server/chat/chat.go` + `cmd/helix-org/chat.go`
all build `mcp.json`, exec `claude`, and handle session-resume
differently (`01 §6 bullet 10`). Introduce `runtime/claude.Session`
that owns mcp.json assembly, `--mcp-config`, session-resume policy,
stream-json parsing. All three sites import it.

**Verify**: zero duplicated `mcp.json`-building code; `--continue` vs
`--resume` decision is in one place.

**Risk**: medium. Owner-chat is currently a long-lived subprocess; AI
Worker spawn is per-activation; CLI chat is `syscall.Exec`. The
`ClaudeSession` type has to support all three lifecycles cleanly.

**Rollback**: keep the three call sites; revert.

#### M10 — Owner UI vs MCP Gateway

Pick one (`04 §2.7`, `04 §4 cut #6`, `01 §6 #4`):

- **Option A (pragmatic)**: ratify the bypass in `CLAUDE.md` — owner
  UI is server-side and trusted; "every mutation goes through MCP"
  is amended to "every Worker-driven mutation goes through MCP".
- **Option B (purist)**: make the owner UI call its own MCP endpoint
  as `w-owner`. Adds one in-process hop; aligns surface with policy.

Pick A unless there's a security argument for B. Document the choice
in ADR-NNNN.

#### M11 — Doc sweep

After M0–M10, edit `CLAUDE.md` and `bootstrap/templates/owner_role.md`
to align with reality:

- Remove "Channels" from structural-primitives list (M0).
- Remove "scope" language from design philosophy (`03 §6 item 3`).
- Replace `--install-claude-mcp` description with the truth (doesn't
  exist) or remove the bullet.
- Replace "`helix-org bootstrap` seeds the owner" with the truth
  (`serve` seeds; `bootstrap` is `helix-runtime preflight`).
- Fix the `--continue` docstring in `cmd/helix-org/chat.go:32`.

---

## Part C — ADRs to write

Decisions worth pinning. One file each in `design/adr/`.

| # | Title | Decides |
|---|---|---|
| 0001 | Terminology pinned | Stream not Channel; Identity not persona/profile; no scope; worker-policy.md not agent.md; Activation as a first-class noun |
| 0002 | The seven bounded contexts | Names + responsibilities + context map per `04` |
| 0003 | Transport parsers live in their transport package | Per M1; future TransportKinds don't touch `domain/` |
| 0004 | Domain events bus | Choice of in-process pub/sub mechanism (start with `broadcast.Broadcaster` repurposed; SQLite outbox later if we want at-least-once) |
| 0005 | WorkerHired event drives Helix project provisioning | Per M4; covers reconciliation policy for failed project creation |
| 0006 | Activation is an aggregate | Per M5; covers ActivationID format, transcript shape, coalescing-window policy |
| 0007 | Message envelope is a typed value object | Per M6 |
| 0008 | Principal value object | Per M6; how transports translate external sender → Principal |
| 0009 | Role markdown contract | Decides between (a) prose-only `## Tools` / `## Streams` (current) and (b) typed `DefaultTools` / `DefaultStreams` fields (chosen — M7) |
| 0010 | Owner UI vs MCP Gateway | Per M10; Option A or B |
| 0011 | Three claude invocation sites consolidated behind ClaudeSession | Per M9 |

---

## Part D — Metrics

Tracked from M1 onward. Numbers per release; report direction, not
absolute thresholds.

| Metric | Definition | How to gather | Target direction |
|---|---|---|---|
| **Cyclic dependencies** | Count of import cycles, *including* the soft cycles dodged by locally-redeclared interfaces (`03 §2.11`) | `go list -deps -json ./...` + manual count of "duplicate interface to dodge a cycle" patterns | → 0 |
| **Core-domain coverage** | `go test -cover` for `org/`, `comms/`, `activation/` (the three core contexts post-M2/M5) | `make test-cover` | ↑ |
| **Prompts co-located with owner** | Count of `//go:embed *.md` directives whose containing package matches the prompt's owning context | grep | All embeds in the right context |
| **Direct external-client touch points** | Number of files importing `helix/helixclient` OR calling `os/exec` of `claude` | grep | → ≤2 (one per Runtime impl, plus tests) |
| **`tools/` cross-context imports** | Number of `tools/*.go` files importing `agent/*` or `helix/*` | grep | → 0 after M4 |
| **Files >500 LOC** | Per `01 §5.2`; SRP smell proxy | `find … wc -l` | ↓ |
| **MCP tools across contexts** | Number of MCP tools per context (Org Graph / Communication / Activation) | manual classification | Stable; split-if-grows-past-15 per context |
| **Doc/code drift** | Count of `CLAUDE.md` claims contradicted by current code (the three from `02 §cross-cutting #2` is the baseline) | manual audit each release | → 0 after M11 |

---

## Part E — What we are deliberately *not* doing

To avoid scope creep, the redesign as planned **does not**:

- Replace SQLite or change persistence. The GORM AutoMigrate story is
  fine; the schema shape is the work.
- Introduce authentication. `CLAUDE.md` defers it; we defer it.
- Add a message broker. The in-process pub/sub is sufficient; the
  EventAppended bus is intentionally lightweight.
- Add retries to outbound webhook/email emit. The 5-second-timeout-then-
  drop semantics (`02 Cap. 3 pain point 4`) is the agreed shape;
  redoing it is a separate project.
- Promote `helix-log.md` to a system construct. Per `05 §8 wrinkle 1`,
  it lives on the LLM's side of the seam by design.
- Lift role-markdown section parsing into code beyond `DefaultTools`
  and `DefaultStreams`. Workflow stays in prose (`CLAUDE.md` design
  philosophy and `05 §8 wrinkle 3`).
- Make the three Dispatcher interfaces obsolete by introducing a
  fourth one. Collapse to one (`03 §6 item 6`); don't add.

These are explicit so reviewers don't ask.

---

## Part F — First two sprints

A realistic concrete two-sprint slice if the work were to start now:

**Sprint 1**
- M0 (ADR-0001 + terminology rename) — 1 day
- M1 (Transport parsers out of domain) — 2 days
- M11 first half (doc sweep for items already true after M0) — 1 day
- M8 (helixclient ISP split) — 2 days
- ADR-0002, 0003 — 1 day

**Sprint 2**
- M2 (Dispatcher split into Scheduler + Outbox) — 4 days
- M3 (Runtime port + registry, remove serve.go switch ladders) — 3 days
- M7 (Role.DefaultTools/Streams + hire invariant) — 2 days
- ADR-0004, 0009 — 1 day

By end of sprint 2: supporting contexts are clean, hire invariants
have teeth, and the three highest-leverage core moves (M4, M5, M6) are
unblocked. Sprints 3+ are M4 → M5 → M6 (the big core refactors)
behind a feature flag.

---

## Closing assessment

The codebase has clearer bones than the user feared. The domain layer
is unusually well-named (`03 §closing`); the prompt layer has a
consistent five-section schema in role.md; the in-process model is
honest about its limits (`01 §3`). The mess is concentrated in three
places, all surfaced repeatedly across this analysis:

1. **`tools/` knows about a specific Runtime backend** (M4).
2. **`Activation` exists as prose but not as a type** (M5).
3. **`CLAUDE.md` describes a system the code doesn't keep up with**
   (M0, M11).

Fix those three and the system is no longer vibe-coded — it's a small
clean DDD-shaped Go service with a deliberate prompt-driven core. The
rest is hygiene.
