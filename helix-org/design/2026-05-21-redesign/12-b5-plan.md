# 12 — B5 plan: Activation as a first-class aggregate

**Status.** Drafted 2026-05-22, immediately after B9 (delete dead claude
invocation sites) landed in `20511c765`.

**Source.** `08-migration-plan.md §M5` + `05-tactical-patterns.md §3`,
re-scoped through `09-integration-reframe.md` (canonical home is
`api/pkg/org/activation/`, not `helix-org/`).

## Why

Today there is **no `Activation` type**. The concept is implicit in:

- the `Spawner` function signature (`api/pkg/org/runtime/runtime.go:Spawner`),
- the per-Worker `workerQueue` in `helix-org/dispatch/dispatcher.go`,
- the convention `s-activations-<workerID>` (`helix-org/agent/prompt.go:178`),
- the inline `=== activation: hire ===` / `=== exit: ok ===` markers in
  the transcript (originally `agent/claude/spawner.go`, now only in
  `api/pkg/org/runtime/helix/spawner.go` + `helix-org/server/chat/helix_bridge.go`).

Consequences (per `05 §3.6`):

1. Every transcript event has no `activation_id` — `worker_log` returns
   the firehose of a Worker's activation stream rather than a single
   turn (`02 Capability 4 pain point 1`, TODO.md item 2).
2. The `ActivationStreamID(workerID)` constructor is in `helix-org/agent/`,
   used by 5 files spread across packages — wrong owner.
3. Activation outcome is a magic string at the end of the transcript
   (`=== exit: ok ===` / `=== exit: error: ... ===`); callers
   string-match to know if an activation completed.
4. Coalescing-window state (when to fold a second trigger into the
   currently-running activation vs. queue a new one) lives in the
   Dispatcher mixed with scheduling concerns (`05 §3.5
   CoalescingQueue`).
5. Two paths create the activation Stream (`hire_worker.go`,
   `bootstrap.go`); the Worker-creation invariant is unenforced.

## End state

```
api/pkg/org/activation/
  trigger.go           # exists (lifted in B3c)
  stream.go            # NEW — StreamID(workerID), MessageBody helper
  outcome.go           # NEW — Outcome VO + Status enum + parser of "=== exit: ===" markers
  segment.go           # NEW — TranscriptSegment VO + parser of "assistant: …" lines
  activation.go        # NEW — Activation aggregate {ID, WorkerID, Triggers, StartedAt, EndedAt, Outcome, TranscriptStreamID}
  repository.go        # NEW — Repository port; sqlite/postgres impls land alongside H4
  queue.go             # NEW — per-Worker activation Queue (extracted from helix-org/dispatch)
```

Behaviour:

- `Activation` row persisted per Spawner invocation. Indexed by
  `(worker_id, started_at desc)`.
- Spawner emits `ActivationStarted` / `ActivationCompleted` domain
  events through the existing pubsub.
- `worker_log` gains `activation_id` filter; without one, returns the
  current behaviour (all activations for the Worker) for back-compat.
- `hire_worker` returns the `ActivationID` of the hire-Activation.
- The `=== ... ===` string markers in the transcript stream become
  redundant; the Activation row carries the same information typed.
  Markers stay for a transitional window so existing readers don't
  break — `Outcome` parses them at read time.

## Risk + rollback

`08 §M5` notes this is **medium-high** risk. The plan stages the work
so each commit is independently revertable. The aggregate lands first;
callers migrate one by one. No feature flag is necessary as long as
each step preserves observable behaviour at the seams that matter
(transcript shape, worker_log output, hire_worker response).

## Sub-steps

Same pattern as H1: each is a focused PR landing canonically under
`api/pkg/org/activation/`. Every file added carries high-level
TDD unit tests written **before** the implementation (red-green) per
`helix-org/CLAUDE.md` "Testing → tdd". Characterisation tests pin
behaviour preserved across lifts.

| # | Title | Effect | Effort |
|---|---|---|---|
| B5.1 | Lift `ActivationStreamID` → `activation.StreamID` | One canonical home for the s-activations-<workerID> derivation. Updates 5 callers. | S |
| B5.2 | `activation.Outcome` VO + parser | Lift the `=== exit: ===` marker shape into a typed VO with `Parse(string)`. Used by `worker_log` and the chat bridge's transcript reader. Markers still get *written* by the Spawner; readers stop matching strings. | S |
| B5.3 | `activation.TranscriptSegment` VO + parser | Lift the `assistant: …` / `tool_use foo: …` / `tool_result: …` line shape. Replaces the string parsing inside `worker_log`. Writers (the helix Spawner's bridge.go + chat helix_bridge.go) stay unchanged. | M |
| B5.4 | `activation.Activation` aggregate + `Repository` port | The struct + a port; no callers yet. Pure type addition. | S |
| B5.5 | `activation.Repository` GORM impl in helix-org/store, with migration | One thin row per Spawner run; AutoMigrate (per `helix-org/CLAUDE.md` "Go → GORM AutoMigrate only"). | M |
| B5.6 | Wire Spawner to create/complete Activation rows | Both `api/pkg/org/runtime/helix.Spawner` and the owner-chat path in `server/chat/helix_bridge.go` create an Activation at start, mark it Completed/Failed at end. Transcript-stream events keep flowing unchanged. | M |
| B5.7 | `worker_log` gains `activation_id` filter | New optional arg; without it, behaviour unchanged. | S |
| B5.8 | `hire_worker` returns ActivationID | Schema change: `hire_worker` MCP response gains `activation_id`. Caller (the chat bridge / Worker prompt) can poll on a single Activation. | S |
| B5.9 | Move activation-Stream creation to Worker creation | Single enforcement site (replaces `bootstrap.go:155` + `hire_worker.go:237`). | S |
| B5.10 | Extract CoalescingQueue from Dispatcher | Lifts the per-Worker burst-folding state into `activation.Queue` (renamed from Coalescer per CLAUDE.md "no -er suffixes"). Closes `05 §3.5`. | M |

## Recommended order

1. **B5.1** first — smallest move, single-symbol lift, zero behaviour
   change. Validates the canonical-home pattern for this package.
2. **B5.2 → B5.3** — VOs land before the aggregate that holds them.
3. **B5.4 → B5.5 → B5.6** — aggregate, then storage, then writers.
4. **B5.7 + B5.8** — surface the ActivationID to consumers.
5. **B5.9 → B5.10** — last because they touch live code paths
   (hire_worker / dispatcher) that other in-flight work might also
   modify; do them once the rest of B5 has stabilised.

## TDD discipline

For each step:

1. Write the failing test in the target package first. Run it,
   confirm `red`.
2. Implement the minimum to make it `green`.
3. Run the full local test set (`make test` in helix-org +
   `go test ./pkg/org/... ./pkg/server/...` in api) before
   committing.
4. Characterisation tests (existing-behaviour pins) come BEFORE any
   lift that moves a function; new TDD tests come BEFORE any new
   type.

## What B5 does NOT do

- Multi-tenant scoping. Activations stay keyed by `worker_id` only;
  the `(org_id, worker_id)` re-keying is H5's job.
- Replacing the transcript stream with the Activation row. The two
  coexist: the stream is the observation surface (chat, MCP), the
  row is the audit/index surface (`worker_log` filter, future
  rate-limit, replay).
- Changing the Spawner port shape. `runtime.Spawner` keeps its
  signature; the Activation aggregate sits *above* the Spawner — the
  dispatcher creates and completes it around the Spawner call.

## Why this plan deviates from `08 §M5`

`08 §M5` proposed a single L-effort PR behind an `activation.v2`
feature flag. With B9 done, we already have direct controller calls
into helix (no helixclient loopback), and the runtime backend is
canonical. So we can land Activation incrementally without a feature
flag — each sub-step preserves observable behaviour. The flag was
proposed when the spawner was external; it isn't now.
