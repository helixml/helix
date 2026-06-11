# Auto-wake cutting short long tool-call sessions

Spec task: [002091 - Stop Auto-Wake From Cutting Short Long Tool-Call Sessions](https://github.com/helixml/helix/blob/helix-specs/design/tasks/002091_the-worker-sessions-are/)

## Problem

Worker sessions that ran any tool call lasting longer than ~60s — common
for docs-writing, `git push`, `npm install`, large file writes — were
being interrupted by a "↻ Retried" / "Incomplete interaction" banner
and re-prompted by the auto-wake worker. Effectively this capped the
practical length of a worker session at 2–3 minutes.

The chat UI showed the agent happily streaming thinking and tool calls
before the interruption, which made the failure look like "the agent
went unresponsive." It wasn't. The agent was working through a tool
call that produced no streamed ACP events; the API saw a silent
WebSocket for >60s and concluded the session was stuck.

## Root cause (two compounding bugs)

### 1. Decoy interaction (load-bearing)

`api/pkg/org/infrastructure/runtime/helix/spawner.go` created a single
`actCtx` with a 5-minute `cfg.ActivationTimeout` and passed it to both
`ensureSession` and `pollUntilDone`. When the timer fired on a healthy
session that just happened to take longer than 5 minutes, the spawner
returned `context.DeadlineExceeded`, the `activation.Queue` released
its per-Worker serialisation lane, and the next pending trigger drained
and spawned a fresh interaction on the still-running session. That new
interaction landed with `state=waiting`, empty `response_message`, NULL
`response_entries`. From `ListStuckWaitingInteractions`'s SQL filter
perspective it was indistinguishable from a real stuck row.

### 2. Wake-up threshold shorter than common tool durations

`defaultAutoWakeStuckThreshold = 60 * time.Second` in
`api/pkg/server/auto_wake_stuck_interactions.go`. The streaming-context
gate at `maybeAutoWake` checked `time.Since(sctx.lastPublish) < 60s`
to skip wake-up while the session is publishing. `lastPublish` is
bumped only when an ACP `session/update` event arrives. The agent emits
`session/update` *around* tool calls (assistant text → tool_call →
tool_result → assistant text), not *during* them. A 90s `git push`
produced zero events for its entire duration; `lastPublish` decayed
past 60s and the gate failed by ~1 second.

## Fix

### Layered by responsibility

- **Org layer** (`api/pkg/org/...`) should serialise activations per
  Worker and call the session API faithfully. It is NOT the org
  layer's job to second-guess whether the session is healthy.
- **Session layer** (`api/pkg/server/auto_wake_stuck_interactions.go`)
  already owns stuck detection and has all the signals (WebSocket
  state, streaming context, `session.Updated`, response progression).

The reviewer caught an earlier iteration that proposed injecting a
`LivenessProbe` into `SpawnerConfig`. That would have pulled
session-layer state into the org layer — wrong direction. Removing the
artificial deadline from the org layer entirely is both simpler and
architecturally cleaner.

### Phase 1 — Session layer

Bumped `defaultAutoWakeStuckThreshold` from 60s → 180s. 180s covers
typical synchronous tool durations (`git push`, `npm install`,
`gh pr view`, `find /`) with 3× margin on the observed ~61s gap.
`HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` still overrides at runtime.

Updated the comment block at `auto_wake_stuck_interactions.go:396-415`
that previously claimed "tool-call cascades and thinking bursts touch
lastPublish on every event, so an actively-streaming session will
reliably stay above the threshold." True for cascades of many short
tools, false for a single long tool — the gate cannot see inside a
synchronous tool execution.

### Phase 2 — Org layer

Renamed `SpawnerConfig.ActivationTimeout` → `SessionStartupTimeout`
(default 5 min, applied to `ensureSession` and pre-session work only)
and added `SpawnerConfig.ActivationRunawayGuard` (default 24h,
applied to `pollUntilDone`). The runaway guard is a pure resource-
safety backstop — not operator-tunable, not a liveness threshold.

Split the contexts in `Spawner`: shared `parentCtx` carries the bearer
token; `startupCtx` (5min) bounds project apply / MCP attach / secret
injection / `ensureSession`; `pollCtx` (24h) bounds `pollUntilDone`.
The Queue lane stays held until the session API reports terminal
status — the correct serialisation behaviour.

## Verification

### Unit tests

- `TestAutoWakeStuckThresholdDefault` / `TestAutoWakeStuckThresholdOverride` —
  pin the 180s default and env-var override (with garbage / zero fallback).
- `TestSpawnerSessionStartupTimeoutBoundsStartup` — hanging `StartSession`
  fires `SessionStartupTimeout` long before the much larger
  `ActivationRunawayGuard` would. Proves startup is bounded by
  `SessionStartupTimeout`.
- `TestSpawnerPollPhaseNotBoundedBySessionStartupTimeout` — poll loop
  runs past the `SessionStartupTimeout` boundary and terminates only
  at `ActivationRunawayGuard`. Direct regression test for the
  decoy-spawning bug.
- `TestSpawnerTimeoutEmitsExitError` — retargeted to exercise
  `ActivationRunawayGuard`.
- All 15 `TestSpawner*` tests pass.
- All `AutoWake*` tests pass.

### E2E

E2E reproduction was deferred to operator post-merge because:

- The inner Helix `api` container hosts the agent doing this work; a
  forced restart would interrupt this and any concurrent worker
  sessions.
- Air's inotify watcher does not fire reliably through the bind mount
  for `api/pkg/org/infrastructure/runtime/helix/` in this setup.

Steps for the operator are in the spec task `tasks.md` Phase 3.

## Files touched

- `api/pkg/server/auto_wake_stuck_interactions.go` — constant bump,
  comment rewrites.
- `api/pkg/server/auto_wake_stuck_interactions_test.go` — fixture
  updates (`-90s` → `-4 * time.Minute`), two new unit tests.
- `api/pkg/org/infrastructure/runtime/helix/spawner.go` — field
  rename, new field, context split.
- `api/pkg/org/infrastructure/runtime/helix/spawner_test.go` — field
  renames in test config, two new focused tests, context-aware
  `startBlock` hook on `fakeHelixClient`.

## Operator notes

- After merge, operators currently running with
  `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=600` (or any value above
  180) can remove the override — the 180s default is now safe for
  tool-call workloads.
- `HELIX_ACTIVATION_TIMEOUT_SECONDS` was not previously documented as
  an operator knob; the rename is invisible to anyone who didn't set
  it. (Quick `grep -rn "HELIX_ACTIVATION_TIMEOUT" .` over the repo
  shows no env-var read — `ActivationTimeout` was only a code-level
  config field.)
- `ActivationRunawayGuard` is intentionally NOT operator-tunable.
  24h is a generous-but-finite cap on a single activation; tuning it
  shorter re-introduces the decoy-spawning failure mode.
