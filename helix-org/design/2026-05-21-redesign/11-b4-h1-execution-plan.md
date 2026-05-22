# 11 — B4 + H1 execution plan

**Status**: ready to execute. Supersedes `10-b4-h1-plan.md` (kept in
git history for the surveyed inventory tables, which are still
accurate as a description of "what helixclient is and who calls it").

This is a prescriptive plan: every commit titled, every test named,
every file path canonical. Read top-to-bottom before starting work;
the first half is decisions, the second half is the slice-by-slice
execution.

---

## 0. Why this exists (and why the draft was insufficient)

The draft (`10-b4-h1-plan.md`) is descriptively sound but
prescriptively wrong on five points the survey re-confirmed below:

1. **"helixclient cannot be deleted entirely"** — wrong. The operator's
   helixclient is a **different package**: `api/pkg/client`
   (verified at `operator/internal/controller/aiapp_controller.go:32`
   and `:48`). Nothing outside `helix-org/` and the two
   `api/pkg/server/helix_org{,_chat}.go` wiring files imports
   `helix-org/helix/helixclient/`. H1's end state is **the whole
   package gone**, all 2181 LOC.

2. **"WorkerHired event bus"** is over-engineering. We have one
   subscriber per runtime (helix-runtime impl, claude-runtime no-op),
   chosen at wiring time. That's polymorphism, not fan-out.
   `helix/CLAUDE.md` (parent rules) and `helix-org/CLAUDE.md`
   (§Software Engineering) both say "no speculative abstractions";
   `helix-org/CLAUDE.md` (§ this file) says polymorphism over `switch`
   on a Kind. A single-method interface is the right shape.

3. **"H7 + H8 are separate symbolic moves"** is wrong post-H7. The
   canonical-location rule in `helix-org/CLAUDE.md` (§Refactored files
   land in `api/pkg/org/`) explicitly collapses Track A and Track B:
   every refactor PR lifts to canonical location simultaneously. H8
   evaporates. There is no separate "move packages" step.

4. **"Characterisation tests pin the current code in place, then move"**
   misses that WorkerProject has **zero tests today** (verified —
   the survey found a tested Spawner via fakeHelixClient but no
   project_test.go). For project.go and workspace.go we need
   intent-capturing tests *first*, against the existing helixclient-
   backed implementation, then preserved verbatim through the lift.

5. **"SubscribeUpdates → pubsub" is the high-risk slice** — true, but
   the draft underplays the simplification. The in-process equivalent
   `pubsub.GetSessionQueue(ownerID, sessionID)` receives **the exact
   same wire payload** the WebSocket handler forwards
   (`api/pkg/server/websocket_server_user.go:103-111` is a passthrough
   subscribe → `conn.WriteMessage(payload)`). `NewEntryStream` keeps
   working unchanged. The reconnect-loop in spawner/bridge can be
   simplified, not preserved verbatim.

---

## 1. Architectural decisions (D1–D6)

### D1 — B4 is an interface, not a bus

Define `runtime.HireHook` as a single-method interface in
`api/pkg/org/runtime/`:

```go
// HireHook runs runtime-side bookkeeping immediately after a Worker
// is created. The hire transaction has already committed by the time
// OnHire is called; an error here is logged but does NOT roll back the
// hire (matches today's behaviour — `agenthelix.SaveHiringUser` failures
// are non-fatal at `helix-org/tools/hire_worker.go:217-222`).
type HireHook interface {
    OnHire(ctx context.Context, workerID worker.ID, hiringUserID string) error
}
```

Two implementations:
- `runtime/helix.HireHook` wraps `SaveHiringUser` (the call that
  exists today as `agenthelix.SaveHiringUser`).
- `runtime/claude.NoopHireHook` returns nil.

Wired into `tools.Deps` (the dep-injection bundle hire_worker reads),
replacing the direct `agenthelix.SaveHiringUser` call.

**Why not an event bus**: one publisher, one subscriber, chosen at
wiring time, no fan-out, no future second subscriber on the horizon.
`helix-org/CLAUDE.md` (§Design Philosophy): *"Write the smallest thing
that works. No speculative abstractions, no optional plumbing that
isn't exercised today."* The bus is plumbing for two subscribers we
don't have.

**Why not a callback `func(...)error`**: future symmetry with other
runtime ports (`Spawner`, `WorkspaceSync`) that *are* interfaces, and
to make claude's no-op explicit and named (`NoopHireHook`) rather
than a `func() error { return nil }` literal at wiring sites.

### D2 — Code lifts to `api/pkg/org/runtime/helix/` **in the same PR** as the helixclient swap

Per `helix-org/CLAUDE.md` (§Refactored files land in `api/pkg/org/`):
*"Every refactor PR lifts its target file(s) into their canonical home
under `api/pkg/org/` directly. … The move *is* the refactor."*

For H1, each slice does **both**: lift the file from
`helix-org/agent/helix/<x>.go` to `api/pkg/org/runtime/helix/<x>.go`
**and** replace helixclient calls with direct controller/store/servicer
calls in the same commit. No separate symbolic move.

Final layout after H1:
```
api/pkg/org/runtime/
  runtime.go              (existing — Spawner, WorkspaceSync ports + HireHook from D1)
  runtime_test.go
  helix/
    workspace.go          (H1.1)
    workspace_test.go
    project.go            (H1.2)
    project_test.go
    spawner.go            (H1.3)
    spawner_test.go
    state.go              (lifted with H1.2; small, no logic change)
    auth.go               (H1.0 — see D4)
    hire.go               (B4 — the HireHook impl)
  claude/
    spawner.go            (lifted separately, not in this PR — out of scope)
    hire.go               (B4 — NoopHireHook; lifted with B4)
```

The chat bridge (`helix-org/server/chat/helix_bridge.go`) is a separate
caller of helixclient session methods. Its target canonical home is
`api/pkg/org/chat/` (TBD package name). For H1.3 we **leave it where
it is** and refactor in place — moving the chat package to canonical is
a follow-up not blocked by this work. The CLAUDE.md rule applies
strictly to files inside H1.x's scope (the agent/helix lifts); for
non-canonical files we refactor without moving and keep them on the
legacy side. Document this exception in commit messages.

### D3 — Tests are written first, at the canonical location, against today's helixclient-backed code

For each slice, the order is:

1. **Write the test** at the canonical path (`api/pkg/org/runtime/helix/<x>_test.go`).
   The test imports today's legacy code via the existing import path
   (`helix-org/agent/helix`), because today's code is still in the legacy
   location. This is fine — `api/pkg/org/` may import `api/pkg/org/...`
   but NOT `helix-org/...`. We get around it during Phase 0 by putting
   the *test file* in `helix-org/agent/helix/` initially, then moving
   the test alongside the code in the lift commit. Two-step move:

   - Phase 0 commit: write `helix-org/agent/helix/<x>_intent_test.go`
     (the new tests). They pass against current code.
   - Lift commit: `git mv` both `<x>.go` AND `<x>_intent_test.go` to
     `api/pkg/org/runtime/helix/`, refactor calls in the same commit.

   This satisfies the canonical-location rule (the END STATE of the
   lift commit is canonical), avoids the no-aliases rule (no shim
   files left behind), and keeps tests green throughout.

2. **Run the test against today's code** (commit it green).
3. **Lift + refactor.** The test must remain green without modification.
   If the test needs to change, the lift is not behaviour-preserving;
   either split the PR (behaviour change first, with new tests) or
   abandon the lift.

### D4 — Per-user identity threading uses `*types.User` in context, not bearer tokens

Today: middleware mints API key → `helixclient.WithBearerToken(ctx, key)`
→ helixclient reads from context → HTTP loopback → helix middleware
re-resolves user from key.

After H1: middleware resolves `*types.User` (it already does for HTTP
handlers) → `helix.WithUser(ctx, user)` (new helper in
`api/pkg/org/runtime/helix/auth.go`) → in-process call sites read user
from context and pass `*types.User` to controller methods that need it.

For the spawner's background flow (no request context):
`SpawnerConfig.BearerForUser func(ctx, userID string) (string, error)`
becomes `SpawnerConfig.UserForHiringID func(ctx, userID string) (*types.User, error)`.
The callback is implemented in `api/pkg/server/helix_org.go` and
looks up the user via the existing store. `HiringUserID` (persisted on
WorkerRuntimeState by B4's HireHook) is the lookup key.

This is a one-time wiring change at the boundary, not a per-call site
refactor.

### D5 — Trust model: in-process callers bypass HTTP-handler validation

The loopback HTTP path today uses the static service API key (admin
privileges) for everything that isn't explicitly per-user. Direct
in-process calls bypass HTTP-handler-level validation (org membership,
idempotency-by-name lookups) by definition.

We accept this. Justification:

- The loopback already bypasses meaningful authz in practice — admin
  service key.
- The org-membership check is **per hiring user**, which we now thread
  via `*types.User` directly. Direct controller calls that take a user
  perform the same check at the controller layer where applicable.
- Idempotency by name (e.g., `ApplyProject` upsert-by-name) needs to
  be replicated at the helix-runtime call site if it isn't at the
  controller layer. Each slice's "validation parity audit" subsection
  enumerates which checks are at handler vs controller and what we
  replicate.

If a check ONLY exists at the HTTP handler layer (not controller, not
store), we replicate it inside the helix-runtime layer — one method,
named after the original handler, so the source of truth is obvious.

### D6 — helixclient is DELETED, not shrunk

Per the survey finding above, `helix-org/helix/helixclient/` has no
callers outside `helix-org/` and the two wiring files. After H1.4 the
package is removed entirely:

```bash
git rm -r helix-org/helix/helixclient/
```

The operator's `api/pkg/client` is unaffected; the wiring files in
`api/pkg/server/helix_org{,_chat}.go` either shrink to almost nothing
(if the constructed-and-injected client was the bulk) or are
restructured to construct the new direct-call dependencies.

EnsureAndSend (in `session_send.go`) and the `EntryStream` parser (in
`patches.go`) are useful pieces that survive — they move to
`api/pkg/org/runtime/helix/` and lose their helixclient dependency.
Specifically:

- `EntryStream` + `Apply` + `Flush` + the splice helpers → `helix/entry_stream.go`.
  No HTTP-bound types; takes raw `[]byte` payloads from pubsub.
- `EnsureAndSend` → `helix/sessions.go`. Calls controller methods
  directly instead of `client.StartChat`/`client.SendToSession`.
- The auth context helpers (`WithBearerToken`/`BearerFromContext`)
  → replaced by `WithUser`/`UserFromContext` in `helix/auth.go` per D4.

---

## 2. Survey re-confirmed (authoritative for this plan)

### helixclient inventory (the work)

| File | LOC | What dies, what survives |
|---|---|---|
| `client.go` | 1308 | Entire `Client` interface + `realClient` impl + auth helpers — all DIE. |
| `client_test.go` | 419 | All DIE (replaced by tests at canonical location, per-slice). |
| `patches.go` | 183 | `EntryStream` + `Apply` + `Flush` SURVIVE, moved to `runtime/helix/entry_stream.go`. The other helpers (`AttachMCPToAppWithHeaders`) die. |
| `patches_test.go` | 125 | Move alongside `entry_stream.go` (rename `entry_stream_test.go`). |
| `session_send.go` | 146 | `EnsureAndSend` + `SendPromptParams` SURVIVE, moved to `runtime/helix/sessions.go`, body rewritten to use controllers. |
| (no test) | — | Add `sessions_test.go` in the lift commit. |

### Call sites by file (the deletion targets)

| File | helixclient methods called | Slice |
|---|---|---|
| `helix-org/agent/helix/workspace.go` | `PutFile` | H1.1 |
| `helix-org/agent/helix/project.go` | `GetProject`, `ApplyProject`, `PutProjectSecret`, `WhoAmI`, `CreateGitRepo`, `AttachRepoToProject`, `CreateBranch`, `PutFile`, `AttachMCPToAppWithHeaders` | H1.2 |
| `helix-org/agent/helix/spawner.go` | `EnsureAndSend`, `GetOutput`, `SubscribeUpdates`, `WithBearerToken` (context); calls `WorkerProject.Ensure` inline | H1.3 |
| `helix-org/server/chat/helix_bridge.go` | `GetSession`, `EnsureAndSend`, `StartChatWithStatus`, `SendSessionMessage`, `SubscribeUpdates`, `StopExternalAgent`, `GetProject` | H1.3 |
| `helix-org/tools/hire_worker.go` | `UserIDFromContext` (no HTTP); imports `agenthelix.SaveHiringUser` | B4 |
| `helix-org/server/mcp.go` | `WithBearerToken`, `WithUserID`, `UserIDFromContext` (no HTTP) | H1.0 |
| `api/pkg/server/helix_org.go`, `helix_org_chat.go` | `New` (constructor) + injection | every slice; final cleanup in H1.4 |

### In-process targets (the replacement targets)

| helixclient call | Replacement | Located at |
|---|---|---|
| `WhoAmI(ctx)` | `*types.User` already in context (D4) | `runtime/helix/auth.go` |
| `ApplyProject(ctx, req)` | `controller.Controller.ApplyProject` if exists, else inline `store.CreateProject`/`UpdateProject` with the handler's idempotency check replicated | `runtime/helix/project.go` |
| `GetProject(ctx, id)` | `store.Store.GetProject(ctx, id)` | direct |
| `DeleteProject(ctx, id)` | `store.Store.DeleteProject(ctx, id)` | direct |
| `PutProjectSecret(ctx, projectID, name, value)` | `store.Store.CreateSecret` or matching controller method | direct; verify in slice |
| `CreateGitRepo`, `AttachRepoToProject`, `CreateBranch`, `PutFile`, `GetFile` | `gitRepositoryServicer` methods (interface at `api/pkg/server/git_repository_servicer.go:13`) | direct |
| `CreateApp`, `GetApp`, `UpdateApp` | `store.Store.CreateApp`/`GetApp`/`UpdateApp` | direct |
| `AttachMCPToAppWithHeaders` | inline `GetApp` → mutate `App.Config.Helix.Assistants[0].MCPs` → `UpdateApp` | `runtime/helix/project.go` helper |
| `StartChat`/`StartChatWithStatus`/`SendSessionMessage` | `controller.Controller.CreateSession` + `SendUpdateMessage` (or whichever methods the HTTP handler at `session_handlers.go:326` delegates to) | direct (verify in slice) |
| `GetSession(ctx, id)` | `store.Store.GetSession(ctx, id)` | direct |
| `GetOutput(ctx, id)` | the controller method behind handler at `session_handlers.go:2507` | direct (verify in slice) |
| `StopExternalAgent(ctx, id)` | the controller method behind handler at `session_handlers.go:2191` | direct (verify in slice) |
| `SubscribeUpdates(ctx, id) <-chan SessionUpdate` | `pubsub.PubSub.Subscribe(ctx, pubsub.GetSessionQueue(ownerID, sessionID), handler)` + late-joiner catch-up snapshot from `streamingContexts` | `runtime/helix/sessions.go` |

The "verify in slice" entries need the helix maintainers' or the
existing handlers' bodies read once before the swap. Each slice's
checklist below includes the verification step.

---

## 3. Phase 0 — Characterisation tests

Goal: pin INTENT, not implementation. Tests drive the public surface
(`WorkerProject.Ensure`, `Workspace.MirrorFile`, `Spawner`, the chat
bridge's `Send`/`History`/`New`/`Switch`, `HireWorker.Invoke`) through
fakes; they must remain green after the H1 refactor changes the
substrate underneath.

### Why this matters

Today's coverage (verified):
- `spawner_test.go` exists with a complete `fakeHelixClient` and good
  scenario coverage. **Reuse** this fake's shape for new tests.
- `helix_bridge_test.go` exists with `fakeChatClient`. Reuse similarly.
- `patches_test.go` covers EntryStream parsing. **Keep.**
- `client_test.go` covers HTTP roundtrip. **Dies with the package.**
- `project.go` — **zero tests**. Write all from scratch.
- `workspace.go` — **zero tests**. Write all from scratch.
- `hire_worker.go` — no direct test; behaviour pinned only through
  end-to-end flows. Write a direct test.

### Phase 0 commits (in order)

**Commit P0.1 — `test(helix-org/tools): characterise hire_worker side-effect order`**

New file `helix-org/tools/hire_worker_test.go`. Tests:

| Test name | Asserts |
|---|---|
| `TestHireWorkerHumanCreatesRowsInOrder` | Worker row, Environment row, Grants (when supplied) inserted in that order; NO activation stream; HireHook called with hiring userID iff context carries one; Dispatcher NOT called. |
| `TestHireWorkerAICreatesActivationStreamAndDispatches` | Same as above PLUS activation Stream + Subscription created; `Dispatcher.DispatchHire` called once with right args. |
| `TestHireWorkerGrantsBeforeDispatch` | Grants land in store BEFORE dispatcher fires (race-free first activation). |
| `TestHireWorkerEnvDirCreated` | `<EnvsDir>/<workerID>/` exists on disk. |
| `TestHireWorkerMissingIdentityRejected` | Empty `identityContent` → error before any row insert. |
| `TestHireWorkerHookCalledWithHiringUserID` | The hire hook (B4 interface) receives `(workerID, "u-alice")` when context has user "u-alice". |
| `TestHireWorkerHookFailureNotFatal` | Hook returns error → hire still returns success; error logged via Dispatcher's audit Stream OR returned as error per current code reading (verify against `hire_worker.go:217-222` — that path actually DOES `return nil, fmt.Errorf("persist hiring user: %w", err)`; today's code IS fatal; the doc comment claims non-fatal but the code is fatal. Adopt the **fatal** behaviour as the contract since it's what the code does. Note discrepancy in commit message.) |

These tests use a `fakeStore` with the existing `store.Store` shape
(or `memorystore` if compatible) and a captured `HireHook`. They
exist before B4 commits to verify the refactor doesn't change behaviour.
At Phase 0 they assert against the existing direct `agenthelix.SaveHiringUser`
call shape — they fail to compile if the imports change.

→ The test FOR THE HOOK (last two rows) is added in B4's commits, not
Phase 0, because the hook doesn't exist yet. Phase 0 captures only
what's verifiable against today's code.

**Commit P0.2 — `test(helix-org/agent/helix): characterise WorkerProject.Ensure`**

New file `helix-org/agent/helix/project_test.go`. Tests
(driving `(*WorkerProject).Ensure` through `fakeHelixClient`):

| Test name | Asserts |
|---|---|
| `TestEnsureFreshAppliesProjectAndPushesFiles` | First call on a virgin Worker → `ApplyProject` called once; `PutProjectSecret` called for `HELIX_ORG_URL` + `HELIX_WORKER_ID`; `CreateGitRepo` + `AttachRepoToProject` + `CreateBranch helix-specs` called; `PutFile` called for each of agent.md / role.md / identity.md; runtime state has ProjectID + AgentAppID + RepoID persisted. |
| `TestEnsureWithPersistedProjectFastPaths` | State has a ProjectID → `GetProject` called once; `ApplyProject` NOT called; `CreateBranch` + `PutFile` for role/identity STILL called (the republish path); function returns persisted IDs. |
| `TestEnsureClearsStateOnGetProject404` | State has ProjectID; fake returns `ErrNotFound` from GetProject → state cleared via `ClearProject`; falls through to full re-apply. |
| `TestEnsureApplyProjectEmptyIDIsError` | Fake returns `ApplyProjectResponse{ProjectID: ""}` → Ensure returns error. |
| `TestEnsureAttachesMCPToAgentApp` | After `ApplyProject`, the fake observes an `AttachMCPToApp` (or equivalent GetApp+UpdateApp pair) with the correct URL and Authorization header. |
| `TestEnsureRespectsPerCallBearer` | Context has bearer "k_bob" → the AttachMCP call uses "Bearer k_bob" headers. Spawner-path scenario. |
| `TestEnsureRolePropagatesFromFirstPosition` | Worker.Positions[0].RoleID points to a Role with content "X" → `PutFile` for role.md has content "X". |
| `TestEnsureSkipsRolePushIfNoPosition` | Worker.Positions empty → `PutFile` for role.md is NOT called (or content empty); other files still pushed. |
| `TestEnsureLogsButDoesNotFailOnRepoCreateError` | `CreateGitRepo` returns error → Ensure still returns nil; `repoID` returned empty; warn logged. |
| `TestEnsureLogsButDoesNotFailOnPutFileError` | `PutFile` returns error → Ensure still returns nil. |

These tests use the existing `fakeHelixClient` pattern from
`spawner_test.go:24-135`. The fake captures call counts and the last
request body for each method. Counter assertions test ordering by
running the test linearly and reading counters in sequence.

**Commit P0.3 — `test(helix-org/agent/helix): characterise Workspace.MirrorFile`**

New file `helix-org/agent/helix/workspace_test.go` (already exists per
the survey — extend with intent tests; the existing tests are 3.2K of
mostly-locking checks).

| Test name | Asserts |
|---|---|
| `TestMirrorFileNoOpWhenRepoNotBound` | State.RepoID empty → returns nil; `PutFile` NOT called. |
| `TestMirrorFileCallsPutFileOnHelixSpecsBranch` | Returns `PutFile` invoked with `branch="helix-specs"`, path=`workers/<id>/.context/<name>`. |
| `TestMirrorFileEmptyWorkerIDError` | workerID == "" → returns error. |
| `TestMirrorFileRejectsTraversal` | name=`"../etc/passwd"` → returns error from `ValidateWorkspaceName`. |
| `TestMirrorFileInvalidatesSessionOnRoleEdit` | name=="role.md" with bound RepoID → `SaveSession(ctx, store, workerID, "")` called (invalidates warm session). |
| `TestMirrorFileInvalidatesSessionOnIdentityEdit` | name=="identity.md" → same. |
| `TestMirrorFileDoesNotInvalidateOnOtherFiles` | name=="notes.md" → SaveSession NOT called. |
| `TestMirrorFileSerialisesPerRepo` | Two parallel calls on same RepoID → one PutFile completes before the other starts (lock test; preserve existing). |

**Commit P0.4 — `test(helix-org/agent/helix): augment spawner_test for SubscribeUpdates parity`**

The existing spawner_test.go covers session lifecycle adequately. Add:

| Test name | Asserts |
|---|---|
| `TestSpawnerSubscribesAndReconnectsOnDisconnect` | Fake's `updatesFactory` returns a channel that closes after one update → bridge re-calls `SubscribeUpdates`; subscribeCalls increments. |
| `TestSpawnerPublishesTranscriptViaEntryStream` | Fake feeds a known `SessionUpdate`; verify `publishActivationEvent` is invoked with the rendered TranscriptBody. |
| `TestSpawnerSendsActivationPromptInResume` | State has SessionID → fake's `SendSessionMessage` (or `StartChat` with `SessionID` set) gets called with the activation prompt. |
| `TestSpawnerOpensFreshOnStaleSession` | State has SessionID; fake returns error from resume; observe fresh StartChat. |

**Commit P0.5 — `test(helix-org/server/chat): augment helix_bridge_test for owner-chat`**

Extend `helix_bridge_test.go` with:

| Test name | Asserts |
|---|---|
| `TestBridgeHistoryFromPersistedSession` | LoadSessionID returns "ses_42" → bridge calls `GetSession` then renders transcript. |
| `TestBridgeSendStartsFreshIfNoSession` | LoadSessionID returns ""; observe StartChatWithStatus called with no SessionID; observe SaveSessionID called with the returned ID. |
| `TestBridgeSendResumesIfSession` | LoadSessionID returns "ses_42"; observe SendSessionMessage (or StartChat with SessionID="ses_42"). |
| `TestBridgeNewClearsAndStopsAgent` | New chat handler → SaveSessionID("") + StopExternalAgent. |
| `TestBridgeWebsocketReconnects` | Fake's update channel closes once → bridge re-subscribes. |

**Phase 0 acceptance gate**: all five commits land. `make test` passes
in `helix-org/`. No production code touched yet.

### Phase 0 fake strategy

For each commit, prefer the EXISTING fake pattern in
`spawner_test.go:24` over inventing a new one. The `fakeHelixClient`
struct already implements all 26 `Client` interface methods with
deterministic returns. Extending it with one or two new captures per
slice (e.g. `attachMCPCalls`, `putProjectSecretCalls`) is enough.

Per `helix-org/CLAUDE.md` (§Testing > Mocks): "Prefer hand-rolled fakes
where the interface is small enough that a mock adds no value." 26
methods is borderline, but the existing fake works — don't generate a
gomock variant just for this.

---

## 4. Phase 1 — B4: decouple hire from helix runtime

Three commits. Total ~250 LOC of changes; mostly plumbing.

### B4.1 — `feat(api/pkg/org/runtime): add HireHook port`

File `api/pkg/org/runtime/runtime.go` gains:

```go
// HireHook runs runtime-side bookkeeping immediately after a Worker
// is created. See ADR-TBD (link from this commit's body) for why this
// is an interface rather than an event bus.
type HireHook interface {
    OnHire(ctx context.Context, workerID worker.ID, hiringUserID string) error
}
```

Plus `NoopHireHook` in the same file (dev runtimes and tests use it):

```go
type NoopHireHook struct{}
func (NoopHireHook) OnHire(context.Context, worker.ID, string) error { return nil }
```

Add `runtime_test.go` (or extend existing): one test that exercises
NoopHireHook returns nil for any inputs.

**Acceptance**: package compiles; `go test ./api/pkg/org/runtime/...` passes.

### B4.2 — `feat(api/pkg/org/runtime/helix): add helix HireHook impl`

New file `api/pkg/org/runtime/helix/hire.go`:

```go
package helix

import (
    "context"

    "github.com/helixml/helix/api/pkg/org/runtime"
    "github.com/helixml/helix/api/pkg/org/worker"
    "github.com/helixml/helix/helix-org/store"
)

// Hire is runtime.HireHook backed by the helix-runtime's
// per-Worker runtime-state sidecar. Persists the hiring user's ID so
// the Spawner can later mint per-user identity for that Worker's
// sessions.
type Hire struct {
    Store *store.Store
}

func (h *Hire) OnHire(ctx context.Context, workerID worker.ID, hiringUserID string) error {
    // Re-uses the existing SaveHiringUser implementation (lifted next commit).
    return SaveHiringUser(ctx, h.Store, workerID, hiringUserID)
}

var _ runtime.HireHook = (*Hire)(nil)
```

(Lifting `SaveHiringUser` itself to this package happens in H1.2 alongside
`state.go`. For B4 we keep the import as `agenthelix.SaveHiringUser`
temporarily — yes, this is an import inversion that violates the
downhill rule, BUT it's transient: H1.2 lifts state.go and the import
flips. Note this exception in the commit message.)

**Better alternative**: lift `state.go` in a B4.2a commit *before*
B4.2 so this import inversion never exists. Recommended: do B4.2a.

**B4.2a — `refactor(api/pkg/org/runtime/helix): lift state.go`**

`git mv helix-org/agent/helix/state.go api/pkg/org/runtime/helix/state.go`,
update package declaration from `helix` to `helix` (same name, no change),
update all callers' imports from `agenthelix` to `runtimehelix`
(or whatever local alias makes sense). Tests come with the lift.
This is a pure rename + import update; no behaviour change.

After B4.2a, B4.2 imports `SaveHiringUser` from its new home with no
inversion.

### B4.3 — `refactor(helix-org/tools): wire HireHook into hire_worker`

`tools.Deps` gains a `HireHook runtime.HireHook` field.
`hire_worker.go` Invoke:

- Imports `runtime "github.com/helixml/helix/api/pkg/org/runtime"`
  (replacing `agenthelix`).
- Replaces lines 216-223:
  ```go
  if uid := helixclient.UserIDFromContext(ctx); uid != "" {
      if err := agenthelix.SaveHiringUser(ctx, t.deps.Store, id, uid); err != nil {
          return nil, fmt.Errorf("persist hiring user: %w", err)
      }
  }
  ```
  with:
  ```go
  if uid := authctx.UserIDFromContext(ctx); uid != "" && t.deps.HireHook != nil {
      if err := t.deps.HireHook.OnHire(ctx, id, uid); err != nil {
          return nil, fmt.Errorf("hire handler: %w", err)
      }
  }
  ```
  (`authctx` is the new home for the context helpers per H1.0 below.
  For B4.3, if H1.0 hasn't landed yet, keep `helixclient.UserIDFromContext`
  — it's a context helper with no HTTP. The import goes away in H1.0.)

- Removes the `agenthelix` import line.
- The `helixclient` import stays until H1.0 (which moves the
  context helpers).

Test: extend Phase 0's `hire_worker_test.go` with the two hook tests
(`TestHireWorkerHookCalledWithHiringUserID`, `TestHireWorkerHookFailureFatal`).
Both pass against the new impl.

Wiring: `api/pkg/server/helix_org.go` constructs a
`helix.Hire{Store: orgStore}` and injects it into `tools.Deps.HireHook`.

**Acceptance**: `helix-org/tools/hire_worker.go` no longer imports
`agenthelix`. All Phase 0 hire tests pass plus the two new hook tests.

---

## 5. Phase 2 — H1: replace loopback with direct calls

Six commits across four slices.

### H1.0 — `refactor(api/pkg/org/runtime/helix): lift auth-context helpers`

Move the four context helpers out of helixclient before any other H1
work. They're not HTTP-bound — pure context-stash helpers — so moving
them first decouples every subsequent slice from helixclient at the
context-helpers level.

New file `api/pkg/org/runtime/helix/auth.go`:

```go
package helix

import (
    "context"

    "github.com/helixml/helix/api/pkg/types"
)

type bearerTokenKey struct{}
type userIDKey struct{}
type userKey struct{}

func WithBearerToken(ctx context.Context, token string) context.Context { /* lifted */ }
func BearerFromContext(ctx context.Context) string                      { /* lifted */ }
func WithUserID(ctx context.Context, userID string) context.Context     { /* lifted */ }
func UserIDFromContext(ctx context.Context) string                      { /* lifted */ }

// WithUser stashes the resolved *types.User. Replaces the bearer-token
// dance for in-process calls after H1: direct controller calls take a
// *types.User; the bearer is only relevant for outbound calls to
// third-party services that we don't have any more.
func WithUser(ctx context.Context, u *types.User) context.Context       { /* new */ }
func UserFromContext(ctx context.Context) *types.User                   { /* new */ }
```

Update callers (`hire_worker.go`, `server/mcp.go`, `server/chat/helix_bridge.go`,
`agent/helix/spawner.go`, `agent/helix/project.go`, the two wiring files)
to import from the new location. Tests for the helpers move with them
(small test).

The old helpers in `helixclient/client.go:516-559` are DELETED in this
commit. helixclient is now smaller by ~50 LOC; it still works because
nothing it does internally needed the bearer-context helpers (the
helpers were exclusively read by external callers, who now get them
elsewhere).

**Acceptance**: `grep -rn "helixclient.WithBearerToken\|helixclient.WithUserID\|helixclient.BearerFromContext\|helixclient.UserIDFromContext"` returns no hits in `helix-org/`. `make test` passes.

### H1.1 — `refactor(api/pkg/org/runtime/helix): lift workspace + replace PutFile with git servicer`

Two-step single commit:

1. `git mv helix-org/agent/helix/workspace.go api/pkg/org/runtime/helix/workspace.go`
2. `git mv helix-org/agent/helix/workspace_test.go api/pkg/org/runtime/helix/workspace_test.go`
3. Replace `Workspace.client helixclient.Client` field with
   `Workspace.git gitRepositoryServicer` (or whatever the servicer
   interface is named — verified during the slice).
4. Replace the `client.PutFile(ctx, repoID, helixclient.PutFileRequest{...})`
   call (workspace.go:89-96) with the equivalent git-servicer call
   (`git.CreateOrUpdateFileContents(ctx, repoID, path, branch, []byte(content), message, author, email)`).
5. Update `NewWorkspace` constructor signature; update the one wiring
   site in `api/pkg/server/helix_org.go:150`.
6. Phase 0 tests (now alongside workspace.go) must remain green
   without modification. The fake changes from `fakeHelixClient` to
   a smaller `fakeGitServicer` (only the methods workspace needs).

**Validation parity audit (H1.1)**:

- The HTTP handler for `PUT /api/v1/git/repositories/{id}/contents`:
  - Auth: bearer token. After H1.0 the in-process equivalent has
    `*types.User` in context if any.
  - Org membership: does the user have access to this repo?
- The git servicer interface: does `CreateOrUpdateFileContents` itself
  check repo ownership? **Read once before the swap**; if not, we
  replicate the check in `Workspace.MirrorFile` (one helper, named
  for the HTTP handler).
- For workspace.go specifically the repo is owned by the helix-org
  service account (or whoever created it via WorkerProject). The
  `*types.User` in context is the hiring user. We may need to swap
  to a "service user" identity for this call. Decide in the slice.

**Acceptance**:
- `helix-org/agent/helix/workspace.go` no longer exists.
- `api/pkg/org/runtime/helix/workspace.go` exists; doesn't import `helixclient`.
- All Phase 0 + new workspace tests green.
- `grep -rn "helixclient" api/pkg/org/runtime/helix/workspace.go` returns nothing.

### H1.2 — `refactor(api/pkg/org/runtime/helix): lift project + replace controller calls`

The biggest single commit. ~600 LOC of changes.

1. `git mv` the file pair (project.go + new project_test.go from Phase 0).
2. Replace `WorkerProject.Client helixclient.Client` with a constructor
   that takes `controller *controller.Controller`, `store *store.Store`,
   `git gitRepositoryServicer`. Or wrap in a smaller per-method interface.
3. Method-by-method swap (using the mapping table in §2):
   - `GetProject` → `store.GetProject` (or controller method if validation needed)
   - `ApplyProject` → custom helper that combines `store.CreateProject` / `UpdateProject` + idempotency lookup; verify if there's a `controller.ApplyProject` first
   - `PutProjectSecret` → `store.CreateSecret` / `UpdateSecret`
   - `WhoAmI` → replaced by `helix.UserFromContext(ctx)` (after H1.0)
   - `CreateGitRepo` → `git.CreateRepository`
   - `AttachRepoToProject` → `git.AttachRepositoryToProject` (or whatever the servicer names it)
   - `CreateBranch` → `git.CreateBranch`
   - `PutFile` → `git.CreateOrUpdateFileContents`
   - `AttachMCPToAppWithHeaders` → in-process helper that does `store.GetApp` → mutate `app.Config.Helix.Assistants[0].MCPs` (need to verify the field path against helix's `types.App` definition) → `store.UpdateApp`

**Validation parity audit (H1.2)** — this is the biggest one. Specific checks to make:

- `ApplyProject`'s idempotency-by-name: the HTTP handler at
  `api/pkg/server/project_handlers.go:2534-2565` (per the survey) does
  org+name idempotency. **Replicate** this exact check at the
  in-process call site. The "Verify before swap" deliverable is a
  paragraph in the commit message that says: "Replicated checks:
  org-membership (X lines, ported), name-idempotency (Y lines, ported);
  Skipped checks: …".

- `CreateGitRepo`'s owner resolution: the handler infers owner from
  request user. The in-process call has `*types.User` from context
  (after H1.0). Pass it explicitly.

- The MCP attachment was a GET-mutate-PUT against `app.Config`. The
  helix-side `types.App.Config.Helix.Assistants[0].MCPs` shape is
  what we mutate. **Read the type definition** before writing the
  helper so we don't drift from the HTTP path's JSON unmarshalling
  behaviour.

**Per-call bearer/user threading**: today `helixclient.BearerFromContext(ctx)`
returns the per-request bearer (chat-bridge path) or per-activation
bearer (spawner path). After H1, the user lives in context as
`*types.User` via `helix.UserFromContext`. Both call sites
(chat-bridge, spawner) need to populate this differently:

- Chat-bridge: middleware (`withHelixUserBearer` in
  `helix_org_chat.go:257`) already resolves the user; just stash the
  full user, not the token.
- Spawner: `BearerForUser(ctx, userID) (string, error)` becomes
  `UserForHiringID(ctx, userID) (*types.User, error)`. One call-site
  change in the wiring file.

**Acceptance**:
- `helix-org/agent/helix/project.go` deleted.
- `api/pkg/org/runtime/helix/project.go` exists; imports no helixclient.
- Phase 0 project tests green.
- `grep -rn helixclient api/pkg/org/runtime/helix/project.go` returns nothing.

### H1.3 — `refactor(api/pkg/org/runtime/helix): lift spawner + sessions, replace SubscribeUpdates with pubsub`

The riskiest commit. ~700 LOC of changes. Split into sub-commits:

**H1.3a — `refactor(...): lift EntryStream + EnsureAndSend to runtime/helix`**

Pure file moves with one rename:

1. `git mv helix-org/helix/helixclient/patches.go api/pkg/org/runtime/helix/entry_stream.go`
2. `git mv helix-org/helix/helixclient/patches_test.go api/pkg/org/runtime/helix/entry_stream_test.go`
3. `git mv helix-org/helix/helixclient/session_send.go api/pkg/org/runtime/helix/sessions.go`
4. Change package from `helixclient` to `helix`.
5. EnsureAndSend currently calls `client.StartChatWithStatus` and
   `SendToSession`. **Leave these for now** — H1.3c rewrites the body
   against in-process. H1.3a is only a file move so the diff is
   reviewable.

EnsureAndSend at this point still depends on a Client interface;
define a temporary in-package mini-interface `sessionClient` (just the
3-4 methods EnsureAndSend uses) and have the legacy `helixclient.Client`
satisfy it. This decouples the file from the package without yet
rewriting the body.

**H1.3b — `refactor(...): substitute pubsub for WebSocket in SubscribeUpdates`**

Add to `runtime/helix/sessions.go`:

```go
// SessionUpdates is a thin adapter over pubsub.Subscribe that returns
// a channel-of-bytes the EntryStream can consume. Replaces the
// loopback WebSocket dial inside helixclient.SubscribeUpdates.
//
// Topic: pubsub.GetSessionQueue(ownerID, sessionID) — identical to
// what api/pkg/server/websocket_server_user.go:103 subscribes the
// browser WebSocket to. The payload is bytes; callers decode via
// the existing EntryStream.Apply (which expects helixclient.SessionUpdate
// shape — verify with a one-line struct copy or a Decode helper).
func SubscribeSessionUpdates(ctx context.Context, ps pubsub.PubSub, ownerID, sessionID string) (<-chan SessionUpdate, error) {
    out := make(chan SessionUpdate, 64)
    sub, err := ps.Subscribe(ctx, pubsub.GetSessionQueue(ownerID, sessionID), func(payload []byte) error {
        var u SessionUpdate
        if err := json.Unmarshal(payload, &u); err != nil {
            return err
        }
        select {
        case out <- u:
        case <-ctx.Done():
        }
        return nil
    })
    if err != nil {
        return nil, err
    }
    go func() {
        <-ctx.Done()
        _ = sub.Unsubscribe()
        close(out)
    }()
    return out, nil
}
```

**Late-joiner catch-up**: the WebSocket handler at
`websocket_server_user.go:128-156` reads `streamingContexts[sessionID]`
and sends a full-state snapshot before the subscription. We need the
in-process equivalent. Two options:

- (a) **Replicate**: expose `streamingContexts` as a method on
  the server / a `SessionPreamble` interface and call it from
  `SubscribeSessionUpdates`.
- (b) **Skip**: in-process subscribers know how to handle missed
  patches because EntryStream is patch-idempotent under snapshot
  replay (`TestEntryStreamSnapshotReplayDoesNotDoubleEmit` already
  pins this).

**Recommendation: (a)** — exposing the snapshot is a small additive
move and removes one class of "first activation drops first patch"
bug. Define a `SessionPreamble` interface and pass it to
`SubscribeSessionUpdates`. The wiring file constructs it from the
HelixAPIServer instance directly (in-process, no abstraction tax).

Test: extend Phase 0's `TestSpawnerSubscribesAndReconnectsOnDisconnect`
to use a fake `pubsub.PubSub` instead of `fakeHelixClient.updatesFactory`.

**H1.3c — `refactor(...): rewrite EnsureAndSend body against controllers`**

Replace `client.StartChatWithStatus` / `SendToSession` / `CheckDesktopQuota`
with direct controller calls. The relevant controllers per the survey:
`controller.Controller.CreateSession` + `SendUpdateMessage`. The exact
sequence:

1. Resolve `*types.User` from context (after H1.0).
2. If `params.SessionID` set, call the controller's
   send-to-existing-session method. The "fall through to fresh on
   error" semantic stays.
3. Pre-flight desktop quota: this is `controller.Controller.CheckDesktopQuota`
   or similar — verify the method exists with that name.
4. Open fresh: `controller.Controller.CreateSession(ctx, user, types.CreateSessionRequest{...})`.
   The `Stream=true` zed_external path that helixclient's
   `startChatStreaming` implemented becomes whatever the controller
   does for stream-mode — verify.
5. The "OnSessionID callback fires the moment Helix echoes the session
   ID" semantic: in-process this is simpler — `CreateSession` returns
   the session synchronously, fire the callback before returning.

The `hadStreamErr` retry semantic (sessions.go:132-144) becomes
trivially achievable in-process (or unnecessary, depending on what
the controller method does on coldstart-race). Verify before
rewriting; if the retry was working around a loopback HTTP race
that doesn't exist in-process, remove it.

Tests: Phase 0's spawner + bridge tests must remain green. Add a
unit test for `EnsureAndSend` itself with a fake `sessionClient`
(in-package interface).

**H1.3d — `refactor(...): lift spawner.go + helix_bridge.go to canonical, swap callers to in-process`**

1. `git mv helix-org/agent/helix/spawner.go api/pkg/org/runtime/helix/spawner.go`
2. `git mv helix-org/agent/helix/spawner_test.go api/pkg/org/runtime/helix/spawner_test.go`
3. Update spawner.go's call sites:
   - `helixclient.EnsureAndSend(...)` → in-package `EnsureAndSend(...)` (no more `helixclient` prefix).
   - `c.Client.GetOutput(...)` → controller method (verify).
   - `cfg.Client.SubscribeUpdates(...)` → `SubscribeSessionUpdates(ctx, ps, ownerID, sessionID)`.
   - `cfg.Client.StopExternalAgent(...)` → controller method (verify).
4. Update `SpawnerConfig` fields: drop `Client helixclient.Client`,
   add `Controller *controller.Controller`, `PubSub pubsub.PubSub`,
   `Snapshotter SessionPreamble`. Rename
   `BearerForUser func(ctx, userID) (string, error)` to
   `UserForHiringID func(ctx, userID) (*types.User, error)` per D4.
5. helix_bridge.go: refactor in place (per D2 — bridge stays in
   helix-org/server/chat/ for now). Same caller-side substitutions.
6. Update the two wiring files (`helix_org.go`, `helix_org_chat.go`)
   to construct the new dependencies (controller, pubsub, snapshotter,
   gitservicer) and remove all `helixclient.New(...)` construction
   sites.

Tests: all Phase 0 spawner + bridge tests green. The
`fakeHelixClient` is replaced by smaller fakes
(`fakeSessionController`, `fakePubSub`) that the new code consumes.

**Acceptance for H1.3 overall**:
- `helix-org/agent/helix/spawner.go` deleted.
- `helix-org/agent/helix/` directory is empty (besides activations.go,
  policy.go, prompt.go, worker-policy.md which are unrelated). Possibly
  the directory can be cleaned up entirely if those are also moved.
- `helix-org/server/chat/helix_bridge.go` no longer imports helixclient
  (it imports the new `runtime/helix` package directly).
- All session lifecycle tests green.
- Manual smoke test: `make test` plus, ideally, a local
  `helix api` + `HELIX_ORG_ENABLED=true` test that drives an
  activation through the new path.

### H1.4 — `chore(helix-org): delete helixclient package`

Pure deletion commit:

```bash
git rm -r helix-org/helix/helixclient/
```

Verify before pushing:

```bash
# Nothing inside the helix repo (apart from possible doc comments) references it
grep -rn "helix-org/helix/helixclient" /home/phil/helix --include="*.go"
# Should return 0 hits
```

`api/pkg/server/helix_org.go` and `helix_org_chat.go` get their
`buildHelixOrgServiceClient` and friends deleted (or reduced to
nothing if a wiring helper still exists for the new dependencies).

The auto-provisioned service API key
(`ensureHelixOrgServiceAPIKey`, `helix_org.go:131`) is no longer
needed for helix-org → helix calls. **Verify** it's not used for
other purposes (e.g., MCP gateway auth) before deletion. If still
used for the helix-org MCP server's bearer requirement, keep that
specific helper and rename it for clarity (`ensureHelixOrgMCPServiceKey`?).

**Acceptance**:
- `helix-org/helix/` is empty (or removed entirely).
- `make test` passes in `helix-org/` and `api/`.
- `make ci` passes (formatting, vet, lint).
- helix-org/CLAUDE.md updated: remove the "two helixclients" diagram
  reference if any; add a line noting `helix-org/helix/` is gone.

---

## 6. Commit-by-commit summary

| # | Title | Phase | LOC est |
|---|---|---|---|
| P0.1 | `test(helix-org/tools): characterise hire_worker side-effect order` | 0 | +200 |
| P0.2 | `test(helix-org/agent/helix): characterise WorkerProject.Ensure` | 0 | +400 |
| P0.3 | `test(helix-org/agent/helix): characterise Workspace.MirrorFile` | 0 | +150 |
| P0.4 | `test(helix-org/agent/helix): augment spawner_test for SubscribeUpdates parity` | 0 | +200 |
| P0.5 | `test(helix-org/server/chat): augment helix_bridge_test for owner-chat` | 0 | +250 |
| B4.1 | `feat(api/pkg/org/runtime): add HireHook port` | B4 | +50 |
| B4.2a | `refactor(api/pkg/org/runtime/helix): lift state.go` | B4 | move |
| B4.2 | `feat(api/pkg/org/runtime/helix): add helix HireHook impl` | B4 | +60 |
| B4.3 | `refactor(helix-org/tools): wire HireHook into hire_worker` | B4 | +/-40 |
| H1.0 | `refactor(api/pkg/org/runtime/helix): lift auth-context helpers` | H1 | move + 30 |
| H1.1 | `refactor(api/pkg/org/runtime/helix): lift workspace + replace PutFile with git servicer` | H1.1 | -130 +80 |
| H1.2 | `refactor(api/pkg/org/runtime/helix): lift project + replace controller calls` | H1.2 | -290 +400 |
| H1.3a | `refactor(...): lift EntryStream + EnsureAndSend to runtime/helix` | H1.3 | move |
| H1.3b | `refactor(...): substitute pubsub for WebSocket subscription` | H1.3 | +120 |
| H1.3c | `refactor(...): rewrite EnsureAndSend body against controllers` | H1.3 | -100 +120 |
| H1.3d | `refactor(...): lift spawner.go + helix_bridge.go, swap callers to in-process` | H1.3 | -600 +400 |
| H1.4 | `chore(helix-org): delete helixclient package` | H1.4 | -2181 |

**Total**: ~17 commits, ~3000 LOC deleted, ~2500 LOC added/moved.
Net deletion ~500 LOC visible + the 2181 from helixclient = ~2700 LOC
net deleted. The "headline win" from `09-integration-reframe.md §4 H1`
("1308 LOC drops to near-zero") is conservatively true; the full
package + tests + helpers brings it closer to 2200.

### Review batching

| Batch | Commits | Effort | Why batched |
|---|---|---|---|
| 1 | P0.1–P0.5 | 1 session | All tests, low review tax, establishes intent before any code moves |
| 2 | B4.1–B4.3 | 1 session | B4 complete, self-contained, demonstrates the interface pattern |
| 3 | H1.0 | 0.5 sessions | Foundation for all H1 work, single concern |
| 4 | H1.1 | 0.5 sessions | Smallest H1 slice, validates the lift+refactor pattern |
| 5 | H1.2 | 1.5 sessions | Largest single slice, reviewer fatigue is a real risk; allow time |
| 6 | H1.3a–H1.3d | 2 sessions | Hardest slice, sub-commits already split |
| 7 | H1.4 | 0.5 sessions | Pure deletion + wiring cleanup |

**Total estimated effort**: 7 sessions / 1.5–2 weeks at one session per
day.

**Critical**: do not merge H1.0–H1.3 if there's a pause in the work
that lets the legacy `helix-org/agent/helix/` and the new
`api/pkg/org/runtime/helix/` co-exist for more than a sprint. Either
finish the slice queue or revert to a stable state.

---

## 7. Risks and mitigations

| # | Risk | Probability | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Controller methods have different validation/error semantics than the HTTP handlers' wrappers | High | Med | Each slice's "Validation parity audit" reads the handler before the swap; replicate any handler-only check inside the helix-runtime caller. |
| R2 | `pubsub.PubSub`'s payload shape differs subtly from `SessionUpdate` wire JSON | Low | High | Phase 0 commits include a payload-equivalence test: feed a real session-queue publish through `EntryStream.Apply` and verify the same events emerge as from a captured WebSocket payload. |
| R3 | Late-joiner catch-up logic (websocket_server_user.go:128-156) is load-bearing and silently dropping it loses the first patch on activation | Med | Med | H1.3b's `SessionPreamble` interface is exposed; spawner calls it before subscribing. Verified via test `TestSpawnerCatchupReceivesSnapshotBeforeStream`. |
| R4 | `StartChatStreaming`'s 10-min detached-context coldstart wait has no in-process equivalent and direct controller calls timeout sooner | Med | Med | Read `controller.CreateSession` before H1.3c. If it has a shorter timeout, parameterise it or wrap the call to honour `cfg.ActivationTimeout` instead. |
| R5 | `withHelixUserBearer` middleware behaviour is subtly different from what direct `*types.User` threading produces (e.g., the bearer was per-organisation, not per-user) | Low | Med | Before H1.0, write one test that verifies a request from "user A in org X" produces the same downstream `*types.User.OrganizationID` whichever path. Mismatch is a fix here, not in the runtime. |
| R6 | The chat bridge's heavy `helix_bridge.go` file (1000+ LOC by sight) doesn't lift to canonical in this work, and refactoring in place creates an "ugly" middle state | Med | Low | Document explicitly in commit message and CLAUDE.md: the chat bridge stays at `helix-org/server/chat/` for H1; canonical move is a separate follow-up. |
| R7 | The `ensureHelixOrgServiceAPIKey` at boot (`helix_org.go:131`) is load-bearing for something we don't notice (e.g., MCP gateway auth) | Med | Med | Before H1.4, `grep` every reference; deletion candidate, but keep a clearly-named helper if MCP gateway still needs it. |
| R8 | A subtle race between the snapshot read and the subscribe (the WebSocket handler subscribes FIRST then snapshots, comment at `websocket_server_user.go:124`) is silently inverted in the in-process path | Med | High | Mirror the original order: subscribe FIRST, then snapshot, in `SubscribeSessionUpdates`. Test pins this. |
| R9 | Hire failure semantics regression: B4.3 makes the hook call FATAL (matching today's actual code) but doc-comments suggested non-fatal. Reviewers expect non-fatal | Low | Low | B4.3 commit message calls this out; if non-fatal is preferred, change in a follow-up commit, not as part of B4 (single behaviour-change per commit). |
| R10 | The legacy code under `helix-org/agent/helix/` accumulates "ghost imports" during the slow lift (some files moved, some not) | Med | Low | Each slice's commit fixes all imports in the same commit. No half-moved state across commits. Verified by `make ci` per commit. |

---

## 8. Verification checklist (final acceptance)

After H1.4 lands:

- [ ] `find /home/phil/helix/helix-org/helix -type f` → empty.
- [ ] `grep -rn "helix-org/helix/helixclient" /home/phil/helix --include='*.go'` → 0 hits.
- [ ] `grep -rln "helixclient" /home/phil/helix/helix-org --include='*.go'` → 0 hits (the prefix `helixclient.` is gone from helix-org code; tools/, server/, agent/ all clean).
- [ ] `find /home/phil/helix/api/pkg/org/runtime/helix -type f -name '*.go' | xargs grep -l 'helix-org/'` → 0 hits (downhill rule).
- [ ] `find /home/phil/helix/api/pkg/org -name '*.go' -newer ee0ecf976 | xargs grep -L '_test.go'` → every non-test file has a sibling test file (per `helix-org/CLAUDE.md` canonical rule).
- [ ] `make test` passes in `api/` and `helix-org/`.
- [ ] `make ci` passes (formatting, vet, lint).
- [ ] Manual smoke: start `helix api` with `HELIX_ORG_ENABLED=true`, hire one Worker through `/ui/`, observe activation runs end-to-end, transcript appears.
- [ ] No regression in the operator: `cd operator && go build ./...` succeeds (the operator's `api/pkg/client` is untouched, but verify).
- [ ] Each `api/pkg/org/runtime/helix/<x>.go` has a sibling `<x>_test.go` with TDD-shaped tests.
- [ ] `helix-org/CLAUDE.md` updated: §Architecture section reflects the new state (no more "two helixclients", no more loopback HTTP).
- [ ] `helix-org/design/2026-05-21-redesign/09-integration-reframe.md §4 H1` row marked done (or a follow-up doc 12-… written as the execution log).

---

## 9. Out of scope / deferred

Explicitly NOT part of this work:

- **Lifting `helix-org/server/chat/helix_bridge.go` to canonical** —
  this needs a target package (`api/pkg/org/chat/`?) that doesn't
  exist yet; deferred to its own design + lift.
- **Lifting `helix-org/agent/claude/`** — the dev runtime stays where
  it is; B4.1 / B4.2 only adds the `claude.NoopHireHook`.
- **Lifting `helix-org/domain/`** — the Worker, Stream, Event, Subscription,
  Environment aggregates stay in helix-org domain for now.
  Per `09-integration-reframe.md §4 H4`, they move when storage moves
  to Postgres.
- **Merging the helix-org `tools` registry with helix's `api/pkg/tools`**
  — per `09 §5 decision 1`, deferred.
- **Multi-tenancy (H5)** — the "one shared owner Worker across all
  gated users" constraint stays; we ship single-tenant after H1.
- **Storage migration (H4)** — SQLite stays. Postgres migration is its
  own H-track slice.
- **The "treat all callers as root" comment in `helix-org/CLAUDE.md:33`**
  — was already dead per the PR #2286 work; documentation cleanup tracked
  separately as B11.

---

## 10. Open questions to resolve mid-flight (not blockers)

1. **Should `runtime.HireHook` extend later to `OnFire` / `OnRoleChange`?**
   Not now. When a second event needs runtime-side reaction, *then*
   we add the method or split to multiple interfaces. (Per D1 reasoning.)

2. **Should EnsureAndSend's `hadStreamErr` retry path survive?** Likely
   not — it was a workaround for a loopback race that no longer exists
   in-process. Decide during H1.3c by reading `controller.CreateSession`.

3. **Does the helix-org MCP gateway still need a service API key?**
   `ensureHelixOrgServiceAPIKey` and the MCPAuthBearer wiring path —
   read once during H1.4 cleanup. Likely yes for the helix-side MCP
   gateway to authenticate inbound MCP calls from runners; rename for
   clarity.

4. **Is there a `controller.Controller.ApplyProject` that already
   wraps the validation?** Survey said handler-level; verify before
   H1.2. If yes, use it. If no, replicate the handler's two
   validation checks (org membership, idempotency-by-name) at the
   runtime-side call site.

5. **`SubscribeSessionUpdates` channel buffer size**: 64 is a guess.
   The WebSocket handler doesn't buffer (writes synchronously). Pick
   a size that matches typical burst (per-token-emit) without
   unbounded growth. 64 is fine; revisit if logs show drops.

---

## 11. Notes for the executor

- **Re-read this doc before starting each slice.** The risks and
  validation audits are slice-specific; reading top-to-bottom each
  time keeps the right context in mind.
- **Tests come first.** Every slice. Don't skip Phase 0; the existing
  intent isn't documented anywhere else.
- **One commit = one observable behaviour change.** If a commit's
  body says "and also fixed X" — split it.
- **Run `make ci` per commit, not per batch.** Catches drift early.
- **When the controller / store / git-servicer surface looks "too
  big" or "weirdly shaped" mid-slice**, that's a signal to stop and
  document a follow-up cleanup, not to redesign the helix-side API.
  Scope creep here will blow up the slice.
- **The `helix-org/agent/helix/` directory should shrink to nothing
  by H1.3d.** If `policy.go`, `prompt.go`, or `activations.go` (small
  helpers) are still there at the end, lift them as a final
  housekeeping commit before H1.4. They were not surveyed because
  they aren't on the helixclient path, but they're in the way of
  the directory becoming empty.

---

## 12. Why this is bulletproof (and where it could still go wrong)

The plan eliminates the four classes of bug the draft was open to:

1. **Behaviour drift** — characterisation tests at canonical location
   in Phase 0 mean we know exactly what we're preserving.
2. **Hidden external coupling** — confirmed (via grep) that
   helixclient has no external callers; the operator's client is a
   different package.
3. **Validation regression** — each slice has an explicit "Validation
   parity audit" subsection that names which handler-layer checks
   need to be replicated.
4. **Reconnect/snapshot logic loss** — the late-joiner catch-up and
   the WebSocket reconnect loop are both called out explicitly with
   their mitigations.

Where it could still fail:

- **The chat bridge (helix_bridge.go) lifts in place, not to canonical.**
  This breaks the spirit of D2 ("canonical from day one"). Justified
  because the bridge's canonical home isn't yet decided. Risk: a
  reviewer pushes back. Pre-empt with the commit message.
- **EnsureAndSend's rewrite (H1.3c)** is where unknown-unknown bugs
  live. The 10-min coldstart wait, the SSE-error-chunk retry — both
  are workarounds for loopback HTTP behaviour that may not translate.
  Mitigation: the slice's first commit is *only* the file move (H1.3a)
  so EnsureAndSend keeps its old body; H1.3c rewrites with focused
  attention.
- **Tests-first works only if the tests are good.** A test that
  asserts "PutFile was called with X" is fragile. Prefer asserting
  on observable post-conditions (state on the Worker, content in the
  test file) over interaction matching where possible.
