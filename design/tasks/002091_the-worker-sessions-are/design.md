# Design: Stop Auto-Wake From Cutting Short Long Tool-Call Sessions

## Summary

Two changes, layered. Either alone reduces the symptom; together they
eliminate it for realistic tool-call workloads.

1. **Hold the activation Queue lane until the session reaches terminal
   status, not until `ActivationTimeout` fires.** Eliminates the decoy
   interaction at its source. This is the load-bearing fix — when there
   is no decoy row, there is nothing for the auto-wake SQL filter to
   match against.
2. **Raise `defaultAutoWakeStuckThreshold` from 60s → 180s and make the
   constant explicit about why.** Defence in depth: even if a decoy
   ever does appear (e.g. from a bug path we haven't found), the gate
   won't false-positive on a 90s tool call.

Both ship together. We do not pick one. The threshold bump is the
quick mitigation; the lane-holding fix is what makes the worker stop
seeing decoys in the first place.

## Decision 1 — Lane held until session-terminal, not actCtx-terminal

### Current behaviour

`Spawner` at `api/pkg/org/infrastructure/runtime/helix/spawner.go:219`
creates a 5-minute `actCtx` and passes it into `pollUntilDone` at
:281. When `actCtx` deadlines, `pollUntilDone` returns
`context.DeadlineExceeded`, `Spawner` returns, `Queue.activate` returns,
`Queue.run` drains the next batch from `pending`. If anything is
pending, the Queue calls `spawn` again, which materialises a new
interaction on the still-running session.

### New behaviour

When `pollUntilDone` returns `context.DeadlineExceeded` AND the live
session is **demonstrably still alive**, the spawner does NOT return.
It extends its deadline and keeps polling.

"Demonstrably still alive" means one of:

- A WebSocket connection to the external agent exists for this
  session (`externalAgentWSManager.getConnection(sessionID).connected
  == true`), AND
- The session has had a streaming-context publish OR a
  `session.Updated` bump within the last `ActivationTimeout`
  (i.e. the agent has produced visible work since we started waiting).

If neither holds, treat the timeout as terminal as today and return.
This preserves the existing behaviour for sessions that are genuinely
hung — we keep the bounded blast radius of the original 5-minute cap.

The extension is implemented as a loop with a per-iteration deadline
equal to `ActivationTimeout`. There is no hard upper bound on total
duration; the loop exits when the session reports terminal status
(`IsTerminalOutput`) or when liveness can no longer be demonstrated
within one extension window.

### Why this is correct

The Queue's contract is "at most one in-flight Spawn per Worker"
(`queue.go:26-29`). The current code violates the *spirit* of that
contract: when `actCtx` fires on a still-running session, a second
spawn lands on top of it. Holding the lane while the session is
provably alive enforces the spirit of the invariant.

The change is local to `pollUntilDone`. The Queue, Dispatcher, and
event-fan-out paths are unchanged. The next pending trigger continues
to coalesce via the existing `pending []Trigger` mechanism in
`workerLane`; it just waits longer before being drained.

### Where the change lives

- `api/pkg/org/infrastructure/runtime/helix/spawner.go`
  - `Spawner` (line 159 onward): change the polling loop, OR
  - `pollUntilDone` (line 478): take an extra liveness probe and a
    `parentCtx` so it can extend its own deadline.

The liveness probe needs to consult:

- `externalAgentWSManager.getConnection(sessionID)` — same gate
  auto-wake uses. The wsmanager lives in `api/pkg/server/` and the
  spawner lives in `api/pkg/org/infrastructure/runtime/helix/`, so we
  cannot import it directly without a layering violation. Inject the
  probe as a `SpawnerConfig.LivenessProbe func(sessionID string) bool`
  and wire it from the API server at construction time, mirroring how
  `Client`, `Store`, `Hub` are already injected.
- The session-streaming-context's `lastPublish` (only meaningful in
  the API server process). Same wiring story — exposed via the
  injected probe.

Sketch:

```go
type LivenessProbe func(ctx context.Context, sessionID string) bool

type SpawnerConfig struct {
    // ...existing fields...
    Liveness LivenessProbe // nil → behave as today (no extension)
}
```

`pollUntilDone` extension logic (conceptual):

```go
for {
    out, err := c.Client.GetOutput(ctx, sessionID)
    if err == nil && IsTerminalOutput(out) { return ... }
    select {
    case <-ctx.Done():
        if errors.Is(ctx.Err(), context.DeadlineExceeded) &&
           c.Liveness != nil && c.Liveness(parentCtx, sessionID) {
            // Renew deadline and continue.
            newCtx, cancel := context.WithTimeout(parentCtx, c.ActivationTimeout)
            ctx, releaseCtx = newCtx, cancel
            continue
        }
        return ctx.Err()
    case <-time.After(delay):
    }
    delay = backoff(delay, c.PollMax)
}
```

The exact refactor — single function vs. two — is an implementation
detail. The behavioural contract is: while the session is alive, the
spawner does not return.

### Cost / risk

- An activation can now run indefinitely on a chatty session. This is
  fine: the Queue serialises per-Worker, so one Worker burning a slot
  doesn't block other Workers; the `MaxInflight` semaphore at
  spawner.go:155 caps global concurrency regardless.
- The audit row's `EndedAt` is recorded later than today (after real
  completion instead of after 5 min). This is *more* correct, not less.
- If `LivenessProbe` returns a false positive (says alive when actually
  dead), we delay the eventual error by one `ActivationTimeout` per
  cycle. Bounded and self-correcting (next probe will say dead).

## Decision 2 — Raise default threshold to 180 s

### Current

`defaultAutoWakeStuckThreshold = 60 * time.Second` at
`api/pkg/server/auto_wake_stuck_interactions.go:210`.

### New

`defaultAutoWakeStuckThreshold = 180 * time.Second`.

### Why 180 and not 300 or 600

- 180s covers the empirically observed gap (~61s in the user's
  report) with a 3× safety margin.
- Covers typical slow-tool durations: `git push` over a slow network
  (5-90s), `npm install` for a small project (30-120s), `gh pr view`
  on a chatty PR (10-40s), a `find` over a large tree (10-60s).
- Doesn't cover *every* possible long tool (a kernel build, a
  multi-GB upload). For those the lane-held fix (Decision 1) is the
  protection — the threshold is a safety net.
- Keeps the worker reasonably responsive when wake-up IS warranted.
  300+ seconds means the user stares at a frozen chat for 5 minutes
  before the worker engages on a genuine ACP-buffer drop.

### Where the change lives

- `api/pkg/server/auto_wake_stuck_interactions.go:210` — change the
  constant.
- The comment block above the constant (lines 190-209) needs an
  update: today it explains "60s targets the dominant failure mode"
  and references the streaming-context gate; rewrite to acknowledge
  that the dominant *false-positive* mode is long synchronous tool
  calls and 180s is calibrated to cover them.
- Update the comment at lines 396-415 that claims "tool-call cascades
  and thinking bursts touch lastPublish on every event" — the user's
  analysis showed this is true for cascades of *many short* tools, false
  for one *long* tool. Document the limitation; do not pretend it's
  not there.

### Operator override unchanged

`HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` continues to override the
default at `autoWakeStuckThreshold()` (line 259). No code change
needed; just verify the test covers the new default.

## What we considered and rejected

### Rejected: bump threshold to 600s only, don't touch the Queue

Treats the symptom. A session running `find /` for 11 minutes is still
broken. And the decoy interactions still exist — they just sit longer
before the worker engages on them. Future operators reading the SQL
filter will wonder why empty waiting rows pile up on healthy sessions.
Fix the root cause.

### Rejected: emit a heartbeat from the spawner side over the WS

We can't. The spawner runs in the helix-api process; the agent runs
in a container. The WS in question is owned by the agent end. We have
no in-band channel to inject keep-alives from our side.

### Rejected: add a separate "session is busy" flag at session.Updated

Could be made to work but duplicates state that already exists. The
WebSocket connection is the canonical "agent end is reachable" signal;
the streaming context's `lastPublish` is the canonical "agent has
spoken recently" signal. Use both, don't invent a third.

### Rejected: change the SQL filter to exclude decoys

You can't reliably exclude a decoy at the SQL layer — by construction
it looks exactly like a real stuck row. The only way to make the
filter ignore it is to never create it.

## Patterns picked up

- `getConnection` returns `(*conn, bool)` — many call sites in
  `auto_wake_stuck_interactions.go` already check both halves. Mirror
  that shape in the new `LivenessProbe` so the wiring reads naturally.
- Constants tunable via env vars use the pattern at lines 259-279:
  read once per call, parse defensively, fall through to the default.
  Do not invent a new pattern.
- `SpawnerConfig` already takes optional callbacks (`BearerForUser`,
  `Mirror`, `Logger`) and degrades to no-op when nil. `LivenessProbe`
  fits the same shape — when nil, `pollUntilDone` behaves exactly as
  today.

## Quick mitigation while the fix rolls out

Operators on affected instances can set
`HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=180` (or higher) immediately
without redeploying. This will not eliminate decoys but will stop the
gate from false-positiving on common tool-call gaps.
