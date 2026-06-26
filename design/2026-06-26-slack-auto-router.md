# Slack auto-router + thread participation

Date: 2026-06-26
Status: implemented + smoke-tested (live dev stack)
Branch: feat/org-slack-auto-router

## Smoke-test results (live API on localhost:8080, org `test`)

Seeded a "Slack-connected" precondition (workspace topic + Automated filter
router + two AI Workers) via SQL, then exercised the **running binary**:

- Fire-triggered reconcile added one managed route per AI Worker
  (`mentions "<name>" .Message.body`), provisioned + `automated`-marked output
  Topics, and subscribed each Worker. A fired Worker got no route.
- Live name-match routing: a message naming `smokealice` reached her route
  Topic + the (subscriber-less) default; `ai-1` (unnamed) got nothing.
- Word-boundary: `smokealicery` did NOT match `smokealice`; `ai-1` matched.
- The auto-router renders in the helix-org **Chart UI** with its
  `unmatched`/per-Worker outputs wired to the Workers.
- Delete cascaded the 3 owned output Topics; a subsequent fire-reconcile did
  NOT recreate the router (sticky-delete).

Not exercised live (covered by unit tests only): **thread-follow** — the
publish API doesn't carry Slack `ts`/`thread_ts`, so `thread_root` is empty
and membership recording/fan-out require a real Slack event.

## Problem

Connecting a Slack workspace to an org currently delivers every message onto a
single workspace-scoped Topic (`s-slack-ws-<connID>`). To get those messages to
a *specific* Worker, an operator must hand-build a filter Processor (the
"router") and wire one Output ("route") per Worker — a predicate that matches
the Worker's name → a Topic that Worker subscribes to. That manual wiring is a
faff and drifts out of sync as Workers come and go.

We want this to happen automatically, with near-zero intervention, while still
being fully customisable and overridable.

## What already exists (and we reuse)

- **filter Processor** (`domain/processor/filter.go`): reads one input Topic,
  has N `Output`s. Each Output = `{Match predicate, destination TopicID}` — i.e.
  *a route*. Match is a Go `text/template` returning truthy. Zero matches drops
  the message; N matches fans out. **This is the router.** We instantiate one,
  we do not reinvent it.
- **reconcile.Reconciler** (`application/reconcile`): event-driven, idempotent
  diff of the channel topology against the reporting graph; invoked by
  hire/fire and at startup (`ReconcileAll`). **This is the pattern our new
  route reconciler mirrors** (composition: a second, independent reconciler).
- **slackWorkspaceTopics.ensure** (`server/helix_org_slack.go`): creates the
  workspace Topic on connect. The auto-router is created right beside it.
- **processing.Runner / dispatch.Dispatcher**: late-bound execution arms
  (`RegisterProcessorRunner`, `RegisterOutbound`). Thread-follow registers the
  same way.

## Design

### 1. Markers (the provenance the feature needs)

Two cheap markers, no schema migration, reusing existing fields:

- `processor.Output.ManagedFor string` (JSON field on the existing Outputs blob
  — **no migration**). Non-empty = "this route is auto-managed for that Worker
  ID". Empty = a manual route the user added. **The route reconciler only ever
  touches `ManagedFor` routes**; manual routes are invisible to it (never
  GC'd, never rewritten). This is the whole answer to "don't touch user edits"
  and "GC routes for dead workers" — one field, keyed by worker.
- **"Automated" = `CreatedBy == processor.SystemActor` ("helix")** — no new
  column. `CreatedBy` already exists (the chart anchor) and is `""` / a Worker
  id for human-made records; automation stamps the `SystemActor` sentinel
  instead. `processor.Processor.Automated()` derives the flag from it; the route
  reconciler and post-routing hook key on it; the API surfaces a computed
  `automated` boolean (the chart leaves the sentinel unanchored, since no Worker
  is named "helix"). Crucially the marker **propagates for free**: the
  processors service already copies a processor's `CreatedBy` onto the output
  Topics it owns, so an automated router's Topics are automatically `helix`-owned
  too — no `Topic.Automated` plumbing needed.

Worker *names are immutable IDs* (`w-alice`), so there is **no fingerprint and
no rename-sync**: the reconciler only reconciles route *presence/absence* keyed
by worker, never route *content*. A user editing a managed route's predicate is
preserved because presence (a route tagged for that worker exists) is all the
reconciler checks.

### 2. Router creation — create-once, sticky-deleted

The auto-router is created **only when the workspace ServiceConnection is first
created** (the create branch of `upsertSlackWorkspace`), never on token-refresh
re-install, and **never by the reconciler**. Deterministic id
`p-slack-router-<connID>`, `CreatedBy=SystemActor` (the automated marker),
`Kind=filter`, input = `s-slack-ws-<connID>`, one default (unconditional,
**disconnected**) route, and `Config={"thread_follow":true}` (on by default —
matches Slack's "everyone in the thread is notified" expectation; toggle per
router in the UI).

It is torn down with the workspace: disconnecting the Slack workspace (which
removes the `s-slack-ws-<connID>` Topic) also deletes `p-slack-router-<connID>`,
cascading its owned output Topics. A later reconnect creates a fresh one.

Because creation is bound to the connect event and the reconciler only maintains
routes *inside an existing* router, deleting the router makes it stay deleted —
no tombstone required.

### 3. Route reconciler (`application/slackrouting`)

A new, independent reconciler (composition — it *uses* the processors service
and the worker/subscription stores; it is not bolted onto reconcile.Reconciler).
For each automated Slack router in the org:

- Desired managed routes = one per **AI** Worker: `Match = {{ mentions "<name>"
  .Message.body }}` (word-boundary, case-insensitive; `<name>` = worker id with
  the `w-` prefix stripped), output = an auto-provisioned, Worker-subscribed
  Topic, `ManagedFor=<workerID>`.
- Diff vs the router's current `ManagedFor` routes:
  - Worker exists, no managed route → **add** route (provision output Topic via
    the processors service, subscribe the Worker).
  - Managed route whose Worker no longer exists → **remove** route (cascades the
    owned output Topic + its subscription). This is the GC backstop; the normal
    worker-delete cascade already drops the *subscription*, the reconciler drops
    the *route + topic*.
  - Manual routes / the default route → untouched.

Invoked by the same triggers as the channel reconciler: hire, fire, and startup
(`ReconcileAll`), plus once at router-creation time.

### 4. Editable processor outputs (refactor)

`processors.Processors.Update` currently freezes Outputs (v1 "delete & recreate"
note). The reconciler needs per-route add/remove. We add output-level operations
to the service so the auto-provision/cascade/cycle-check invariants stay in one
place:

- `AddOutput(orgID, procID, OutputSpec) (Output, error)` — provisions the owned
  output Topic (or wires an explicit one), appends, re-cycle-checks, persists.
- `RemoveOutput(orgID, procID, topicID)` — drops the Output, deletes the owned
  Topic.

Done TDD. Existing `Update`/`Create` semantics unchanged.

### 5. Domain-event log (`domain/domainevent` + store)

A generic, append-only audit log of **decisions** — distinct from the runtime
`streaming.Event` data plane (named `domainevent` precisely to avoid that
collision). v1 is deliberately minimal: one table, append + query, one emitted
type, no subscribers / projections / replay.

```
DomainEvent{ ID, OrgID, Type, Subject, Worker, Source, Metadata json, CreatedAt }
```

- `Subject` = the entity the event is keyed on (here: the Slack `thread_root`).
- `Worker` = the Worker the decision concerns (the participant).
- `Source` = what decided (the router's processor id).
- Indexed on `(org_id, type, subject)` for the membership read.

Thread membership becomes a **projection**: "distinct `Worker` for
`(org, type='slack.thread_participant', subject=thread_root)` within the last N
days (7)". Nothing is reaped for correctness — the time window is a query bound,
not a delete policy. Optional pruning of old rows is a later, size-only concern.

### 6. Thread-follow

Slack users expect everyone already in a thread to keep receiving replies even
when their name isn't repeated. Two routing modes on the automated router:

1. **Name-match** (stateless) — the filter Outputs (§3). First contact.
2. **Thread-follow** (stateful, new, gated by `Config.thread_follow`) — once a
   Worker is in a thread, every later message in that thread reaches them.

`thread_root` = `Message.ThreadID` if set else `Message.MessageID` (a top-level
Slack message *is* a potential thread root).

Execution stays out of the pure `Process`: the `processing.Runner`, after
running a processor whose `Automated` is true, calls a late-bound
`ThreadFollower` hook (registered like the other arms; defined in `processing`,
implemented in `slackrouting`, so `processing` stays Slack-agnostic). The
follower:

1. Maps the name-matched results back to their `ManagedFor` Workers and
   **records** `(thread_root → Worker)` for each (idempotent: skip if already a
   member). Naming a new Worker mid-thread pulls them in here.
2. If `thread_follow` is on: queries existing members of `thread_root`, and
   **publishes** the message to the managed-route output Topic of every member
   *not* already name-matched (those are delivered by the normal result publish).

Purity preserved: `Process` only does name-match; all state lives in the
follower + the domain-event log.

## Non-goals (v1)

- No event-sourcing framework: no subscribers, projections engine, or replay
  over the domain-event log.
- No mutable per-processor KV store (this is an append-only *fact* log).
- No human-Worker routing (AI Workers only — only they activate).
- No @mention routing (Slack mentions resolve to the app, not the Worker's
  name; we match the Worker's name in text).
- No backfill of routers for pre-existing workspaces; no router auto-recreate.
- No domain-event-log retention/pruning job (window-bounded reads make it
  unnecessary for correctness).

## Build order (TDD each)

1. Editable processor outputs (`AddOutput`/`RemoveOutput`).
2. Markers (`Output.ManagedFor`, `Processor.Automated`, `Topic.Automated` +
   gorm migration).
3. `mentions` word-boundary template func.
4. Domain-event log package + store.
5. `slackrouting` route reconciler.
6. Router creation on connect.
7. Thread-follow arm + Runner hook + toggle.
8. Wire reconciler into hire/fire/startup.
9. Build, go test, UI smoke test on localhost:8080.
