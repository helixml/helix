# 10 — B4 + H1 working plan

**Status**: draft for review. Not yet executed. Successor of
`08-migration-plan.md §B M4` and `09-integration-reframe.md §4 H1`
with concrete survey findings.

This is a *working* plan — written as a scratchpad to be edited
further by hand before any code lands. The eight other docs in this
directory are descriptive (what is, what should be); this one is
prescriptive (what we will do, in what order, with what risks).

---

## Headline finding: helixclient cannot be deleted entirely

The Kubernetes operator at `operator/internal/controller/{aiapp_controller,
project_controller}.go` uses `helixclient.Client` as a *real* HTTP
client to talk to helix from outside the helix process. This is the
correct use of helixclient — different deployment, real wire protocol.

**H1's scope is "delete helixclient from the embedded helix-org
path", not "delete helixclient."** The operator caller stays; the
loopback callers (running inside the helix process) go away. The
surviving subset of helixclient methods may be smaller (~5–8
methods the operator actually uses, versus the current 21).

This reframes the LOC win: it's not "delete 1308 lines of client.go,"
it's "delete all loopback usage from helix-org's embedded path *and*
shrink helixclient to the operator-only subset." Net likely still
~1000+ LOC deleted, just spread across more files.

---

## Survey: what helixclient is and who calls it

### Surface area

```
helix-org/helix/helixclient/         total 2181 LOC
  client.go                          1308   // type Client interface + REST impl
  client_test.go                      419
  patches.go                          183   // higher-level helpers (AttachMCPToApp etc.)
  patches_test.go                     125
  session_send.go                     146   // EnsureAndSend + NewEntryStream wrappers
```

### Client interface — 21 methods, 5 conceptual groups

| Group | Methods |
|---|---|
| **Identity** | `WhoAmI`, `ServerStatus` |
| **Models** | `ListProviders`, `ListModelsForProvider` |
| **Projects** | `ApplyProject`, `GetProject`, `DeleteProject`, `PutProjectSecret` |
| **Repos + files** | `CreateGitRepo`, `AttachRepoToProject`, `CreateBranch`, `PutFile`, `GetFile` |
| **Apps** | `CreateApp`, `GetApp`, `UpdateApp` |
| **Sessions** | `StartChat`, `StartChatWithStatus`, `SendSessionMessage`, `GetSession`, `GetOutput`, `SubscribeUpdates`, `StopExternalAgent` |

Plus from `session_send.go`: `EnsureAndSend` (Start+Send fused),
`NewEntryStream` (parses streamed event lines).

Plus from `patches.go`: `AttachMCPToAppWithHeaders` (adds an MCP
entry to an App's tools list).

### Callers inside the embedded helix-org path

| File | Methods used | Notes |
|---|---|---|
| `helix-org/agent/helix/project.go` | `ApplyProject`, `GetProject`, `CreateGitRepo`, `AttachRepoToProject`, `CreateBranch`, `PutFile`, `CreateApp`, `GetApp`, `UpdateApp`, `PutProjectSecret`, `AttachMCPToAppWithHeaders` | Heaviest user. The whole "ensure a Helix project + AgentApp + git repo for this Worker" flow. |
| `helix-org/agent/helix/workspace.go` | `PutFile`, `GetFile` | role.md / identity.md mirror onto helix-specs branch. |
| `helix-org/agent/helix/spawner.go` | `EnsureAndSend`, `NewEntryStream`, `SubscribeUpdates`, `StopExternalAgent`, `GetSession` | Session lifecycle for each activation. |
| `helix-org/server/chat/helix_bridge.go` | Same session methods as spawner | Owner-chat session. |
| `helix-org/tools/hire_worker.go` | (imports for `WorkerProject.Ensure` and `SaveHiringUser`) | The B4 DIP violation. |
| `helix-org/server/mcp.go` | `WithBearerToken`, `WithUserID`, `BearerFromContext`, `UserIDFromContext` | Auth-context helpers. No HTTP. |
| `api/pkg/server/helix_org.go` | Constructs the loopback `helixclient.Client` and injects it. | Wiring. |
| `api/pkg/server/helix_org_chat.go` | Same wiring + chat bridge | Wiring. |

### Callers outside the embedded path (NOT touched by H1)

- `operator/internal/controller/aiapp_controller.go`
- `operator/internal/controller/project_controller.go`

These are the legitimate cross-process HTTP users. helixclient survives for them.

---

## B4 and H1 are independent — not strictly paired

The earlier docs (`09 §4`) wrote them as a paired unit for narrative
LOC-win reasons. On closer inspection they target different things:

- **B4** is a layering fix: `tools/hire_worker.go` knows the helix
  runtime exists (DIP violation `04 §4 cut #1`). Decoupling makes
  hire pure org-graph.
- **H1** is a substrate swap inside `agent/helix/`: replace 21
  loopback-HTTP calls with direct Go calls on
  `api/pkg/controller.Controller`. Same operations, different wire.

They can land in either order. Sequencing as B4 → H1 gives the
cleanest end-state (after B4 there's one fewer caller for H1 to deal
with), but they're not technically dependent.

---

## B4 — `WorkerHired` event

### Goal

`tools/hire_worker.go` stops importing `agent/helix` and
`helix/helixclient`. Hire becomes pure org-graph:

1. Insert rows (Worker, Environment, Grants, activation Stream,
   Subscription).
2. Emit `WorkerHired{WorkerID, HiringWorker, EnvPath, HiringUserID}`.
3. Return success.

Helix-runtime-side provisioning moves to an `OnWorkerHired`
subscriber.

### New surface

- `api/pkg/org/events/` package (new) holding:
  - `WorkerHired` struct.
  - A tiny in-process synchronous fan-out bus
    (`Bus.Subscribe(handler) Subscription`,
    `Bus.Publish(ctx, event) error`).
  - Maybe later: `RoleUpdated`, `IdentityUpdated`, `WorkerFired`.
    Don't speculate now — add events when callers exist.

### Bus shape: synchronous, in-process, fail-fast

- **Synchronous**: subscribers run in the publisher's goroutine. The
  first subscriber error bubbles up to the publisher (hire_worker)
  and fails the hire. Matches current behaviour where
  `WorkerProject.Ensure` is called inline before hire returns.
- **In-process**: no NATS, no payload-serialisation, no cross-process
  delivery. Same rationale as the H2 broadcast decision.
- **Fail-fast**: better UX for "did hire succeed?" than
  eventual-consistency. If runtime provisioning fails, the row
  inserts can be rolled back (or the Worker can be marked
  provisioning-failed for retry).

### Subscribers after B4

| Package | Reaction |
|---|---|
| `runtime/helix.OnWorkerHired` | `WorkerProject.Ensure(workerID)` + `SaveHiringUser`. Moves out of hire_worker. |
| `runtime/claude.OnWorkerHired` | No-op. (Claude needs nothing at hire time; the env dir is created by hire_worker.) |
| (optional) `activation.OnWorkerHired` | Create activation Stream + subscribe hiring Worker. Currently inline in hire_worker. **Skip in B4; do as part of B5.** |

### Scope for B4

- **(small)** Decouple `WorkerProject.Ensure` + `SaveHiringUser`
  only. Leave activation-stream creation inline. ~150 LOC of moves +
  ~80 LOC for the bus. **Recommended.**
- **(medium)** Also move activation-stream creation. Cleaner end
  state but bundles with B5's territory.

### Characterisation needed before B4

Pin `hire_worker`'s current side-effect order through a fake
`WorkerProject`:

1. Worker row inserted.
2. Environment row inserted.
3. Grants inserted.
4. Activation Stream + subscription inserted.
5. WorkerProject.Ensure called (with the right WorkerID).
6. SaveHiringUser called (with the right user ID).
7. Dispatcher.DispatchHire called.

The methodology rule says characterisation tests pin behaviour before
the lift; here the "behaviour" is the side-effect sequence.

### Risks

- The event bus is new infrastructure. Has to be process-local,
  synchronous, in-process, simple. No NATS. Probably ~50 LOC.
- Subscriber-ordering across multiple subscribers for the same
  event matters: helix runtime should run before activation-stream
  creation? Or doesn't matter? Need to think about whether ordering
  is part of the contract.
- After B4, `tools/hire_worker.go`'s import graph drops two heavy
  dependencies. Confirm no transitive imports surprise us.

### LOC impact

Small — net ~−50 / +200. Not the headline win; this is plumbing
that enables H1's win.

---

## H1 — replace loopback with direct controller calls

### Goal

Every `helixclient.Client` call site inside the embedded helix-org
path replaced with a direct call on `api/pkg/controller.Controller`,
`api/pkg/store.Store`, or related in-process services.

### Caller-by-caller mapping (rough)

| Caller (file) | helixclient method(s) | In-process equivalent |
|---|---|---|
| `agent/helix/workspace.go` | `PutFile`, `GetFile` | `git_repository_servicer` directly (api/pkg/server/git_repository_servicer.go) |
| `agent/helix/project.go` | `ApplyProject`, `GetProject`, `CreateGitRepo`, `AttachRepoToProject`, `CreateBranch`, `PutFile`, `CreateApp`, `GetApp`, `UpdateApp`, `PutProjectSecret`, `AttachMCPToAppWithHeaders` | `controller.Controller` project methods + `store.Store.CreateApp/UpdateApp/GetApp` + git-servicer + an in-process MCP-attach helper |
| `agent/helix/spawner.go` | `EnsureAndSend` (Start+Send fused), `NewEntryStream`, `SubscribeUpdates`, `StopExternalAgent`, `GetSession` | `controller.Controller.CreateSession`/`SendUpdateMessage` + `controller/controller_external_agent.go` + `pubsub.PubSub` for `SubscribeUpdates`-equivalent |
| `server/chat/helix_bridge.go` | Same session methods as spawner | Same as spawner |
| `server/mcp.go` | `WithBearerToken`, `WithUserID`, `BearerFromContext`, `UserIDFromContext` | These are pure auth-context helpers. **Move** to `api/pkg/org/runtime/auth.go` or similar; not delete. |
| `api/pkg/server/helix_org{,_chat}.go` | Constructs the loopback client | Construct nothing — pass controller/store/services directly into helix-org/agent/helix constructors. |

### Auth-context plumbing

Today the hiring-user bearer token flows through `context.Context` via
`helixclient.WithBearerToken`. After H1, controller methods take
`*types.User` directly (or read it from context).

Options:
- **Keep context-based**: minimal change to call sites. Auth helpers
  move to `api/pkg/org/runtime/`. Controllers that take `*types.User`
  get it from context via a small adapter. **Recommended.**
- **Refactor every caller to pass `*types.User`**: more explicit but
  way more call-site churn.

### Session lifecycle is the hardest part

`SubscribeUpdates` returns `<-chan SessionUpdate` backed by a
WebSocket. The in-process equivalent uses `pubsub.PubSub`
subscriptions on `GetSessionQueue(ownerID, sessionID)`.

Wiring this requires `agent/helix` to import `api/pkg/pubsub`. That's
a downhill-rule violation in spirit (helix-org importing helix's
NATS-based pubsub for runtime needs). Acceptable here because:
1. The session-update channel IS produced by helix's pubsub today —
   no alternative.
2. `agent/helix` is going to be lifted to `api/pkg/org/runtime/helix/`
   in a future H-track migration; at that point the import becomes
   sibling-to-sibling.

The H2 finding that "broadcast is the right shape for in-process
wake-only" doesn't conflict here — session updates have payload, so
broadcast isn't a candidate.

### Scope for H1: four slices

| Slice | What it covers | Effort | Risk |
|---|---|---|---|
| H1.1 — workspace | `PutFile`/`GetFile` in `agent/helix/workspace.go` | S | Low — pure file I/O on the helix-specs branch |
| H1.2 — project | `ApplyProject`, `CreateGitRepo`, `CreateApp`, `PutProjectSecret`, `AttachMCPToAppWithHeaders`, etc. in `agent/helix/project.go` | M-L | Medium — server-side validation in `ApplyProject` may not exist on the direct controller path |
| H1.3 — sessions | `StartChat`, `SendSessionMessage`, `SubscribeUpdates` in `agent/helix/spawner.go` + `server/chat/helix_bridge.go` | L | High — websocket→pubsub translation, retry/reconnect semantics |
| H1.4 — cleanup | Delete the now-unused helixclient methods. Whatever remains is what the operator uses. | S | Low |

Each slice is a separate commit. Sequential: 1 → 2 → 3 → 4.

### Risks (general)

1. **Validation parity.** Controller methods may have different
   error semantics, transaction boundaries, or auth checks than the
   HTTP surface. Each slice needs characterisation tests against the
   current loopback behaviour before the swap.
2. **`SubscribeUpdates` reconnect.** The loopback's WebSocket has
   reconnect-on-disconnect logic. Pubsub-direct may not. Read
   `helixclient.SubscribeUpdates` carefully before slice 3.
3. **`ApplyProject` server-side normalisation.** May do
   server-side validation/normalisation that direct store calls
   bypass. Either go through the controller (which preserves
   validation) or replicate it.
4. **`session_send.go`'s `EnsureAndSend`** is a complex
   higher-level helper. May want to keep it (and rewrite it on top
   of direct controller calls), or fold it into the caller.

### LOC impact

Large — net delete of ~1000+ LOC from helixclient + replacement
code in callers. Headline LOC reduction of the whole redesign.

---

## Proposed commit sequence

Eight commits across B4 + H1, each ≤ ~300 LOC of changes:

| # | Title | Migration | Effort |
|---|---|---|---|
| 1 | `feat(api/pkg/org/events): introduce in-process WorkerHired bus` | B4 | S |
| 2 | `test(...): characterise hire_worker side-effect order through fakes` | B4 | S |
| 3 | `refactor(api/pkg/org): hire_worker emits WorkerHired event (B4)` | B4 | M |
| 4 | `test(...): characterise PutFile/GetFile loopback before H1.1` | H1.1 | S |
| 5 | `refactor(api/pkg/org): replace workspace helixclient with direct git-servicer (H1.1)` | H1.1 | M |
| 6 | `refactor(api/pkg/org): replace project helixclient with direct controller (H1.2)` | H1.2 | L |
| 7 | `refactor(api/pkg/org): replace session helixclient with direct controller + pubsub (H1.3)` | H1.3 | L |
| 8 | `chore(helix-org/helix/helixclient): delete dead loopback methods (H1.4)` | H1.4 | S |

Total estimated effort: **2–4 sessions**, much of it characterisation
tests.

### Suggested batching for review

- **Batch 1**: commits 1–3 (B4 complete). Review pause.
- **Batch 2**: commits 4–5 (H1.1 complete). Review pause.
- **Batch 3**: commit 6 alone (H1.2 — project is non-trivial).
- **Batch 4**: commits 7+8 (H1.3+H1.4 — sessions + cleanup).

---

## Open questions to decide before starting

1. **Bus shape**: synchronous fan-out (subscribers run in caller's
   goroutine, failures bubble up) vs async? **Recommended:
   synchronous.** Matches today's behaviour.

2. **Where does the events bus live?** **Recommended: new
   `api/pkg/org/events/` package.** Different concept from broadcast
   (payload-bearing vs wake-only) so a new package is right.

3. **Slice order for H1**: as proposed (workspace → project →
   sessions → cleanup). Open to reorder.

4. **Auth-context plumbing**: keep context-based (helixclient's
   `WithBearerToken` shape) or refactor every caller to pass
   `*types.User` explicitly? **Recommended: keep context-based.**

5. **`SubscribeUpdates` reconnect logic**: if it has load-bearing
   retry behaviour, do we replicate it in the pubsub-direct path or
   accept the regression? Need to read the current implementation
   carefully before slice 3 starts.

6. **`session_send.go`'s `EnsureAndSend`**: keep as a thin wrapper
   that calls the controller directly, or inline into the two
   call sites? **Recommended: keep as a wrapper** — it's a tested
   composition that's worth preserving.

7. **What to do with operator's helixclient usage**: the operator
   only uses ~5–8 of the 21 methods. Should H1.4 (or a follow-up)
   trim helixclient to just the operator-needed surface, or leave
   the full surface in place for any future external clients?

8. **Failure semantics after B4**: if `OnWorkerHired` (helix runtime
   provisioning) fails, does the hire roll back? Today
   `WorkerProject.Ensure` failure surfaces as hire failure but
   doesn't roll back the inserted rows — the Worker exists in the
   DB without a Helix project. Is this OK to preserve, or do we
   want transactional rollback as part of B4?

---

## What I'd recommend now

Start with **B4 alone** (commits 1–3). It's self-contained, smaller
than any H1 slice, and the WorkerHired event mechanism is reusable
infrastructure that the rest of the redesign will lean on (B5
Activation aggregate needs lifecycle events; future H-track
migrations may want others).

After B4 lands and gets a review pause, proceed with H1 slices in
order.

---

## Notes for future-me / future-reviewer

- The B3 commit (`5435f196a`) already lifted the Runtime port skeleton
  (`runtime.Spawner`, `runtime.WorkspaceSync`) but deferred a unified
  `Runtime interface` because the helix runtime constructs them
  separately with different dependencies. H1 is the right time to
  unify them — once the helix runtime is talking to controllers
  directly, building one constructor that returns one `Runtime`
  satisfying both is straightforward.
- The `agent/helix/` package itself will eventually lift to
  `api/pkg/org/runtime/helix/`. That's a separate H-track migration
  (H8 in `09 §4`, now subsumed into the canonical-location rule's
  "every refactor lands in api/pkg/org/" — meaning the helix
  runtime's lift happens as part of H1.2 or as its own follow-up).
- Don't lift agent/helix to api/pkg/org/runtime/helix/ until H1
  finishes — moving the package while changing its imports
  simultaneously is harder than doing them in sequence.
