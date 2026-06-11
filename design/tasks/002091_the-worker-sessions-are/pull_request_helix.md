# fix(api,org): stop auto-wake cutting short long tool-call sessions

## Summary

Worker sessions running any tool call longer than ~60s (docs-writing,
`git push`, `npm install`, large file writes) were being interrupted by
a "↻ Retried" / "Incomplete interaction" banner and re-prompted. The
agent was healthy — the chat UI was showing the rendered transcript of
events that already arrived — but the API saw no streamed ACP events
during the tool's execution and concluded the session was stuck.

Two compounding bugs, fixed at the right layer each:

1. **Org layer**: `Spawner.ActivationTimeout = 5 min` was applied to
   both `ensureSession` and `pollUntilDone`. When the timer fired on a
   long-running healthy session, the per-Worker `activation.Queue`
   lane released and the next pending trigger spawned a fresh "decoy"
   interaction on top of the still-running session. That decoy
   (`state=waiting`, empty `response_message`, NULL `response_entries`)
   matched the auto-wake worker's SQL filter perfectly.
2. **Session layer**: `defaultAutoWakeStuckThreshold = 60s` is shorter
   than typical synchronous tool durations. The streaming-context
   gate's `lastPublish` decayed past 60s during a normal 90s tool call,
   and the gate failed by ~1 second.

## Changes

### `api/pkg/org/infrastructure/runtime/helix/spawner.go`

- Renamed `SpawnerConfig.ActivationTimeout` → `SessionStartupTimeout`
  (default 5 min). Applied only to `ensureSession` and pre-session
  work (project apply, MCP attach, secret injection).
- Added `SpawnerConfig.ActivationRunawayGuard` (default 24h, not
  operator-tunable). Applied only to `pollUntilDone`. Pure
  resource-safety backstop, not a liveness threshold.
- Split the context in `Spawner` body: shared `parentCtx` carries the
  bearer token; `startupCtx` bounds startup; `pollCtx` bounds the
  poll loop. Lane stays held until the session API reports terminal
  status — correct serialisation behaviour.
- Org layer no longer makes any decision about session liveness. That
  responsibility lives at the session layer.

### `api/pkg/server/auto_wake_stuck_interactions.go`

- `defaultAutoWakeStuckThreshold`: 60s → 180s. Covers typical
  synchronous tool durations with 3× margin on the observed ~61s gap.
- Rewrote the comment block above the constant to document why 180s
  was picked and what the empirical false-positive mode was.
- Rewrote the comment block at `maybeAutoWake`'s streaming-context
  gate to retract the claim that "tool-call cascades touch lastPublish
  on every event" — true for cascades of short tools, false for a
  single long tool.

### Tests

- Two new focused unit tests on `Spawner` pin the startup/poll split:
  - `TestSpawnerSessionStartupTimeoutBoundsStartup` — hanging
    `StartSession` fires `SessionStartupTimeout` before the much
    larger `ActivationRunawayGuard` would.
  - `TestSpawnerPollPhaseNotBoundedBySessionStartupTimeout` — poll
    loop runs past `SessionStartupTimeout` boundary and terminates
    only at `ActivationRunawayGuard`. Direct regression test for
    the decoy-spawning bug.
- `TestSpawnerTimeoutEmitsExitError` retargeted at
  `ActivationRunawayGuard`.
- Two new unit tests on `autoWakeStuckThreshold()` pin the 180s
  default and the env-var override path.
- Two cold-start tests' `Created` fixtures bumped from `-90s` to
  `-4 * time.Minute` so they still clear the new threshold.

### Docs

- `design/2026-06-11-auto-wake-tool-call-fix.md` captures root-cause
  analysis, why the layering matters, the rejected `LivenessProbe`
  approach, and test coverage.

## Verification

- `go build ./api/pkg/org/... ./api/pkg/server/...` — clean.
- All 15 `TestSpawner*` tests pass (including the two new ones).
- All `AutoWake*` tests pass.
- **E2E verified in inner Helix** (after restarting api + frontend to
  pick up the changes):
  - Startup log confirms Phase 1 live:
    `[AUTO_WAKE] Started auto-wake worker ... stuck_threshold=180000`.
  - Created spec task #000001 with a `sleep 200 && echo "tool finished"`
    tool-call payload. Agent ran the full 200-second sleep.
  - Throughout ~10 minutes of session lifetime: exactly one interaction
    row, `auto_wake_count=0`, transitioned cleanly from `state=waiting`
    → `state=complete`. **No decoy interaction ever spawned**, including
    well past the old 5-minute `ActivationTimeout` boundary.
  - Zero `AUTO_WAKE` log entries in the 12-minute test window.
  - UI shows the agent's final reply (`"It printed: tool finished"`)
    with no "↻ Retried" or "Incomplete interaction" banner.
  - Screenshots in
    `design/tasks/002091_the-worker-sessions-are/screenshots/`.

## Operator notes

- Operators currently running with
  `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=600` (or any value above
  180) as a manual mitigation can drop the override after merge —
  180s is the new safe default.
- `ActivationTimeout` was a code-level config field with no documented
  env-var binding (verified via `grep -rn HELIX_ACTIVATION_TIMEOUT
  .`). The rename is invisible to operators.
- `ActivationRunawayGuard` is intentionally NOT operator-tunable.
  24h is generous-but-finite; tuning it shorter re-introduces the
  decoy-spawning failure mode.

## Spec task

[002091](https://github.com/helixml/helix/blob/helix-specs/design/tasks/002091_the-worker-sessions-are/) —
design.md, requirements.md, tasks.md and the reviewer-flagged
relayering of the fix.
