# Design: Finish Agent Questions (ACP Elicitations) — OpenAPI, Tests, E2E and Live Verification

This is a **completion** task. The architecture is already decided and implemented; see
`helix/design/2026-08-11-agent-questions-elicitation.md` (committed, 150 lines) and
`helix-specs/design/tasks/002731_implement-the-feature/design.md`. This document records
**what is already there**, **what is actually missing** (verified by reading the branch, not
assumed), and **how to close each gap**.

## 1. Map of the committed code — read this before touching anything

### Zed — `origin/feature/002731-end-to-end-agent`, commit `8fbf40ad92`

| File | What it holds |
|---|---|
| `crates/external_websocket_sync/src/types.rs` (+125) | `SyncEvent::{ElicitationRequested,ElicitationResolved,ElicitationResync,ElicitationResponseAck}` and their `to_outgoing_message()` arms; `elicitation_status_str()` (`Canceled` → wire `cancelled`) |
| `crates/external_websocket_sync/src/thread_service.rs` (+397) | Emission from `AcpThreadEvent::ElicitationRequested` / `ElicitationResponded` / `EntryUpdated`; the 15 s heartbeat; the reconnect/`open_thread` full resync; the GPUI drain task calling `AcpThread::respond_to_elicitation` |
| `crates/external_websocket_sync/src/external_websocket_sync.rs` (+81) | `ElicitationResponseRequest` + `GLOBAL_ELICITATION_RESPONSE_CALLBACK` + pending queue |
| `crates/external_websocket_sync/src/websocket_sync.rs` (+26) | `respond_elicitation` command dispatch + `handle_respond_elicitation` parser |

**The local `zed` working copy is on `main`.** Step zero is `git status` → stash if dirty →
`git checkout feature/002731-end-to-end-agent` (per CLAUDE.md's branch-switch rule).

### Helix — `feature/002731-end-to-end-agent`, commit `9ddbd5d74`

| File | What it holds |
|---|---|
| `api/pkg/types/agent_elicitation.go` | `types.AgentElicitation` (+ `LastSeenAt`, `IsLive()`) |
| `api/pkg/store/store_agent_elicitations.go` | 8 Postgres methods: `UpsertAgentElicitation`, `GetAgentElicitation`, `TransitionAgentElicitation`, `TouchAgentElicitations`, `ListLiveAgentElicitationsForSession(s)`, `HasLiveAgentElicitation`, `ReapStaleAgentElicitations` |
| `api/pkg/store/store.go:713-720` | Those 8 on the `store.Store` interface; mocks regenerated in `store_mocks.go` |
| `api/pkg/server/websocket_external_agent_elicitation.go` (562 lines) | `handleElicitationRequested/Resolved/Resync/ResponseAck`, `resolveElicitationTarget`, `writeElicitationEntry`, `finalizeElicitation`, `startElicitationReaper`, `notifyAgentQuestion`, `clearAgentQuestionNotification`, `publishSessionElicitationUpdate`, `elicitationResyncGrace()` |
| `api/pkg/server/session_handlers.go:2435-2588` | `respondToElicitation` + `listSessionElicitations` **with swagger annotations** |
| `api/pkg/server/server.go:1059-1060` | Both routes registered |
| `api/pkg/server/websocket_external_agent_sync.go:3443` | `processPromptQueue` dispatch-instead-of-defer gate |
| `api/pkg/server/auto_wake_stuck_interactions.go:235` | Gate 0 — skip interactions blocked on a question |
| `api/pkg/server/wsprotocol/accumulator.go` | `ElicitationEntry` carried through patches + `RestoreAccumulator` |
| `frontend/src/components/session/elicitationSchema.ts` + `.test.ts` | Generic JSON-Schema → fields parser, with 292 lines of Vitest already written |
| `frontend/src/components/session/ElicitationCard.tsx` / `ElicitationCardContainer.tsx` | The card |
| `frontend/src/services/elicitationService.ts` | Calls the **not-yet-generated** client methods |

## 2. Verified gaps

These were confirmed by reading the branch, not inferred.

### Gap 1 — the generated API client is missing the endpoints (blocks `yarn build`)

`grep -c "v1SessionsElicitations" frontend/src/api/api.ts` → `0`. The swagger annotations
exist; the generator has simply never been run. Fix: `./stack update_openapi`, commit
everything it regenerates. Do not hand-edit `api.ts` — the whole point of the CLAUDE.md
rule ("add swagger annotations → `./stack update_openapi` → use generated method") is that
the file is reproducible.

Expected generated names, from the route
`/api/v1/sessions/{id}/elicitations/{elicitation_id}/respond`:
`v1SessionsElicitationsRespondCreate(id, elicitationId, body)` and
`v1SessionsElicitationsDetail(id)`. If the generator produces different names, **change
`elicitationService.ts` to match the generator**, never the reverse.

### Gap 2 — no Go tests exist

`ls api/pkg/server/ | grep -i elicit` → only the implementation file. New file:
`api/pkg/server/websocket_external_agent_elicitation_test.go`.

Style, copied from `websocket_external_agent_sync_test.go`:

```go
type ElicitationSuite struct {
    suite.Suite
    ctrl   *gomock.Controller
    store  *store.MockStore
    server *HelixAPIServer
}
func TestElicitationSuite(t *testing.T) { suite.Run(t, new(ElicitationSuite)) }
func (s *ElicitationSuite) SetupTest() { /* ctrl, MockStore, NewTestServer-ish wiring */ }
```

gomock, not testify/mock. The mocks for all 8 store methods already exist in
`store_mocks.go` — no `mockgen` run needed.

Two decisions worth stating:

- **Race tests are deterministic, not goroutine-timed.** "Two clients answer at once" is
  expressed as two sequential calls where the mock's `TransitionAgentElicitation` returns
  `(true, nil)` on the first call and `(false, nil)` on the second — that *is* the semantics
  of the conditional `UPDATE … WHERE status IN (…)`. Do not spawn goroutines and sleep.
- **The grace window is injected, not slept through.** `elicitationResyncGrace()` reads
  `HELIX_ELICITATION_RESYNC_GRACE_SECONDS` (default 60 s). Resync tests set that env var to
  a small value (or drive `reapStaleElicitations` directly with a synthetic cutoff) rather
  than waiting 60 s.

### Gap 3 — `memorystore` will nil-panic (blocks the *existing* e2e, not just the new phase)

`api/pkg/store/memorystore/memorystore.go` embeds a **nil `store.Store`** (line 28,
deliberately: "panics on unimplemented methods (gives clear stack trace)"). It implements 64
methods; none of them is an `AgentElicitation` method. This compiles fine and explodes at
runtime.

`processPromptQueue` calls `Store.HasLiveAgentElicitation` at
`websocket_external_agent_sync.go:3443`, and the e2e's **existing** Phase 16/17 queue phases
drive `processPromptQueue`. So the committed branch very likely already panics the e2e
before any new phase is added. This is the highest-risk unknown in the task and should be
checked first, by running `./run_docker_e2e.sh` on the branch **as-is** before writing the
new phase.

Fix: add in-memory implementations of the 8 methods to `memorystore` — a
`map[string]*types.AgentElicitation` plus a mutex, mirroring the Postgres semantics that
actually matter:

- `TransitionAgentElicitation` returns `(false, nil)` unless the current status is in
  `fromStatuses` (this is what makes the race and answer-after-cancel tests real).
- `ReapStaleAgentElicitations` filters on `LastSeenAt < olderThan` **and** live status.
- `HasLiveAgentElicitation` filters on `interaction_id` + live status.

### Gap 4 — no e2e phase, and the phase number is taken

Phases 1–17 already exist; 16 = queue-defer, 17 = queue-interrupt (`main.go:1644`). The new
phase is therefore **Phase 18**, appended at the end of `runQueuePhases`, replacing the
`d.phase = 17` terminal state with an 18 that then advances to the next round.

**Why synthetic.** The e2e must not depend on the model deciding to call
`AskUserQuestion` — that is a model-behaviour coin flip, and CI phases that depend on one
are flaky by construction. Phase 10 already establishes the precedent and the seam:
fabricate the sync event and hand it to the real production handler via
`d.srv.ProcessSyncEvent(sessionID, &types.SyncMessage{EventType: ..., Data: ...})`. The
phase comment must say this explicitly.

Shape of Phase 18, using the live thread/session established by earlier phases:

1. `ProcessSyncEvent("elicitation_requested")` with a realistic non-empty `requested_schema`
   (mirror the real adapter: `oneOf` options with `_claude/askUserQuestionOption` metadata
   and an `_askUserQuestionCustomAnswer` sibling), the live `acp_thread_id`, an
   `entry_index`, and `status: "pending"`.
2. Assert the row is live: `srv` store shows the elicitation, and the schema round-tripped
   non-empty (this is the "assert a non-empty schema" requirement — it guards against the
   schema being dropped or stringified by the accumulator).
3. Drive `respond_elicitation` through the **production** path (the same call the REST
   handler makes), and assert the command reaches Zed. Real Zed does not hold this synthetic
   elicitation, so it will reply `elicitation_response_ack{not_found}` — the phase must
   tolerate and log that rather than fail on it (see Open Question 2).
4. `ProcessSyncEvent("elicitation_resolved")` with `status: "accepted"`; assert the row is
   terminal and the transcript entry mirrors it.
5. Send a normal `chat_message` on the same thread and assert `message_completed` — proving
   the turn is not wedged after an elicitation cycle. (This is CLAUDE.md's "test the next
   operation, not just the state change".)

`run_docker_e2e.sh` needs `e2e-test/zed-binary`, which does not exist here — build it first
with `./stack build-zed dev` from the helix repo and copy it. Poll for completion in a loop;
**do not `sleep 540`** (the previous attempt lost a turn that way).

### Gap 5 — `sandbox-versions.txt`

Currently `ZED_COMMIT=eae9b051e17440aa8a9e6bb4035277dcc5d926d3`, which predates
`8fbf40ad92`. Bump it to the final zed commit — taken from a local `git rev-parse HEAD`
*before* pushing zed, per the CLAUDE.md ordering below.

### Gap 6 — no live verification has ever been done

This is the part that actually proves the feature, and the part unit tests cannot stand in
for. CLAUDE.md is explicit: a unit test asserting a state change is not evidence the feature
works, and lifecycle changes must be exercised against a **live, connected Zed**.

Procedure:

1. Register at `http://localhost:8080` (`test@helix.ml` / `helixtest`), complete onboarding
   (org → project). Poll `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080`
   until `200`; the stack takes 5–10 min to come up — do not conclude it is broken early.
2. Create a **spec task** (not a bare chat session — a spec task provisions a git repo, so
   Zed's workspace setup completes and the sync WebSocket opens). Liveness check:
   `config->>'zed_thread_id'` is a non-empty UUID.
3. The session must use the **Claude Code** harness — `AskUserQuestion` is a Claude Code
   built-in, so a `zed-agent`/qwen session cannot produce an elicitation at all.
4. Provoke a question by asking something genuinely ambiguous with named alternatives, e.g.
   *"I want to add caching here. Ask me which backend to use before you write any code."*
   If the model will not call the tool, fall back to injecting the sync event against the
   live session and say plainly in the report that the model-driven path was not observed.
5. Screenshot via `mcp__helix-desktop__{list_windows,focus_window,save_screenshot}` into
   `screenshots/`, or `mcp__chrome-devtools__take_screenshot`.

Failures are reported as failures. Do not report the feature as done on the strength of unit
tests.

## 3. Ship order (from CLAUDE.md — exact, and it matters)

1. Commit in `zed`. **Do not push yet.**
2. `git rev-parse HEAD` in `zed`.
3. Bump `ZED_COMMIT` in `helix/sandbox-versions.txt` to that hash; commit.
4. Open the **Helix** PR first.
5. Push the zed branch; `gh pr create --repo helixml/zed` (the `--repo` flag is mandatory —
   the upstream `zed-industries/zed` remote makes `gh` target the wrong repo otherwise).
6. CI green on both (`gh pr checks`, or the Drone MCP tools — never scrape credentials).
7. Merge **zed first**, then helix. Rebase the `ZED_COMMIT` bump if it moved.

The order exists because the spec-task system marks a task done when all its PRs merge; if
zed merges first, the task can close before `sandbox-versions.txt` is updated, leaving CI
pinned to the wrong commit indefinitely.

## 4. Working constraints learned from the failed previous attempt

- **Commit and push after every meaningful chunk.** The prior sandbox lost its agent
  connection permanently and took a six-hour uncommitted turn with it.
- **Never blind-`sleep` on a build.** Poll in a loop with a short interval so a build that
  finished — or died — is noticed immediately.
- **Prefer targeted builds.** `cd api && go build ./pkg/server/ ./pkg/store/ ./pkg/types/`
  and package-scoped tests, not full-workspace builds. The box is shared and slow; a cold
  `go build` here spent minutes just downloading modules.
- **CGo is required for `go test ./pkg/server/...`** (tree-sitter):
  `sudo apt-get install -y gcc libc6-dev`, then `CGO_ENABLED=1 go test ...`.
- Use `http://localhost:8080` (local dev stack) for all testing — **never** `api:8080`,
  which is the outer production stack.

## 5. Notes for future agents cloning this spec

- The `entry_index` == accumulator `message_id` equivalence is *statically* proven
  (`message_id` is literally the index into `AcpThread::entries()`), but has never been
  confirmed against a live log. Confirm it during live verification.
- The adapter's `_meta` keys are `_askUserQuestionCustomAnswer` and
  `_claude/askUserQuestionOption` as of claude-agent-acp **0.66.0** — *not* the
  `claudeCode/*` names in older briefs. The parser deliberately matches on value *shape*
  (`isCustomAnswer`, or a `questionId` naming a sibling) so another rename does not break
  it. Re-read `claude-agent-acp/dist/elicitation.js` in the deployed version before
  changing any folding logic.
- **"Decline" is not an abort.** It yields `{action:"answered", answers:{}}`; the tool call
  succeeds and the turn continues. Hence the UI label is **"Skip"**.
- `memorystore` embedding a nil `store.Store` means any new `store.Store` method is a
  latent runtime panic in the e2e. Whenever the interface grows, check memorystore.
