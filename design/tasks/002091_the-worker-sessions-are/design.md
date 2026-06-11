# Design: Stop Auto-Wake From Cutting Short Long Tool-Call Sessions

## Summary

Two changes, each landing at the correct layer.

1. **Org layer (`api/pkg/org/...`): stop deciding sessions are stuck.**
   The 5-minute `ActivationTimeout` on the spawner's poll loop is what
   causes the lane to release on a still-running session, which spawns
   a decoy interaction. Remove that timeout from the poll loop. The org
   layer's job is to serialise activations per Worker and call the
   session API faithfully — it is not the org layer's job to second-
   guess whether the session is healthy.
2. **Session layer (`api/pkg/server/auto_wake_stuck_interactions.go`):
   raise `defaultAutoWakeStuckThreshold` from 60s to 180s.** Stuck
   detection lives here. Today it false-positives on common tool-call
   gaps because the threshold is shorter than typical tool durations.

Both ship together. They are independent — either alone would help,
but together they cleanly separate concerns:

- Org layer: "I serialise; I trust the session API."
- Session layer: "I detect stuck sessions and wake them; I own that
  policy."

## Why stuck detection does not belong at the org layer

The `Queue` invariants in `api/pkg/org/domain/activation/queue.go`
are simple: at most one in-flight `Spawn` per Worker; triggers that
arrive during a spawn coalesce into the next batch; distinct Workers
run independently. None of those invariants require the org layer to
know what "stuck" looks like.

The current `ActivationTimeout = 5 * time.Minute` at
`api/pkg/org/infrastructure/runtime/helix/spawner.go:147` is an
embedded liveness assumption: "any healthy activation completes within
5 minutes." That assumption is wrong (docs-writing, large bundles,
slow remote pushes all exceed it) and it leaks into the org layer in
the worst possible way — by releasing the per-Worker serialisation lane
mid-session.

The right place to ask "is the session stuck?" is the session layer.
The session layer already does this via the auto-wake worker, which
has access to all the signals (WebSocket connection state, streaming
context, `session.Updated`, response-message progression). The org
layer has none of those signals and should not be in the business.

So the fix isn't "make the org layer's stuck detection smarter." It's
"stop making the org layer do stuck detection at all."

## Decision 1 — Remove the artificial poll-loop deadline

### Current behaviour

`Spawner` at `spawner.go:219` creates a single `actCtx` with the
5-minute `ActivationTimeout`. That same context is used for both
`ensureSession` (which should be quick) and `pollUntilDone` (which can
run for the entire length of a real worker task). When the deadline
fires on a slow-but-healthy `pollUntilDone`, the spawner returns
`context.DeadlineExceeded`, the Queue releases the lane, and any
pending trigger drains and spawns a decoy interaction on top of the
still-running session.

### New behaviour

Split the contexts:

- **`ensureSession`** keeps a bounded timeout. Bringing a session up
  (creating the row, dialing the WS, applying project state) should
  not take 5 minutes; if it does, that genuinely is a failure and we
  should surface it. Keep the 5-minute budget here.
- **`pollUntilDone`** no longer uses an artificial deadline. It polls
  until `IsTerminalOutput(out)` returns true, the parent context is
  cancelled, or a runaway guard fires (see below). Individual
  `GetOutput` requests retain their own request-level timeouts via
  the HTTP client — network hangs are bounded per-call, not per-loop.

### What about the genuinely-stuck case?

This is the question that motivates having a timeout at all. Two
answers:

1. **Stuck sessions surface via the session layer, not the org layer.**
   When `auto_wake_stuck_interactions` decides a row is stuck and
   retries hit `autoWakeMaxRetries = 2`, the interaction is marked
   `state=error`. The session's `GetOutput` then reports a terminal
   status; `pollUntilDone` sees it and returns naturally. The org
   layer doesn't need its own clock — the session layer's terminal
   transition is the signal.

2. **A runaway guard prevents unbounded resource pinning in the worst
   case.** Cap `pollUntilDone` at something genuinely runaway (e.g.
   24 hours, via a renamed `cfg.ActivationRunawayGuard`, default
   `24 * time.Hour`). This is *not* a stuck-detection knob; it is a
   "no activation should ever take a calendar day" backstop, equivalent
   to the `autoWakeMaxRetries = 1000` runaway-loop backstop already in
   the codebase. The Queue lane releases only on terminal status from
   the session API or on the backstop firing — both of which are
   legitimate end-of-life signals.

### Rename, don't repurpose

`cfg.ActivationTimeout` becomes:

- `cfg.SessionStartupTimeout` (default `5 * time.Minute`) — applied to
  `ensureSession` only.
- `cfg.ActivationRunawayGuard` (default `24 * time.Hour`) — applied to
  `pollUntilDone` only.

Renaming makes the intent explicit at the call site and prevents a
future change re-conflating them. Operators who currently override
`HELIX_ACTIVATION_TIMEOUT_SECONDS` (if any) get a deprecation log line
and the value applied to `SessionStartupTimeout`; the runaway guard is
not operator-tunable in this change — it should never need to be.

### What this avoids

- No new `LivenessProbe` injected from the API server into the org
  layer. That was the wrong direction — it would have moved more
  session-layer state into the org layer instead of less.
- No cross-layer coupling between `org/infrastructure/runtime/helix`
  and `server/externalAgentWSManager` / `server/streamingContexts`.
  The two layers stay independent: the org layer calls the session
  API; the session API encapsulates session liveness.

### Where the change lives

- `api/pkg/org/infrastructure/runtime/helix/spawner.go`
  - Rename `SpawnerConfig.ActivationTimeout` → `SessionStartupTimeout`,
    add `ActivationRunawayGuard`.
  - Update the default-population block at lines 146-148.
  - `Spawner` body: build two contexts. One short-deadline for
    `ensureSession`; one long-runaway-guard for `pollUntilDone`.
  - `pollUntilDone` signature unchanged; only the context handed to
    it changes.
- Any test / production wiring that sets `ActivationTimeout`: rename
  to `SessionStartupTimeout`. Audit `grep -rn "ActivationTimeout"
  /home/retro/work/helix/api/`.

## Decision 2 — Raise auto-wake threshold 60s → 180s

This is the session-layer fix. It does not change *where* stuck
detection happens; it tunes the worker that already owns it.

### Current

`defaultAutoWakeStuckThreshold = 60 * time.Second` at
`api/pkg/server/auto_wake_stuck_interactions.go:210`.

### New

`defaultAutoWakeStuckThreshold = 180 * time.Second`.

### Why 180

- Covers empirically observed gaps (~61s in the user's report) with a
  3× safety margin.
- Covers typical slow-tool durations: `git push` over a slow network
  (5-90s), `npm install` for a small project (30-120s), `gh pr view`
  on a chatty PR (10-40s), a `find /` (10-60s).
- Doesn't cover *every* possible long tool (kernel build, multi-GB
  upload). With Decision 1 in place there's no decoy row for those
  cases, so the worker has nothing to mis-fire on.
- Keeps the worker responsive when wake-up IS warranted (claude-agent-acp
  buffering the tail of a turn). 300+ seconds means the user stares at
  a frozen chat for 5 minutes before the worker engages.

### Where the change lives

- `api/pkg/server/auto_wake_stuck_interactions.go:210` — change the
  constant.
- Rewrite the comment block above (lines 190-209) to acknowledge that
  the dominant *false-positive* mode is long synchronous tool calls,
  and 180s is calibrated to cover them.
- Update the comment at lines 396-415 that claims tool-call cascades
  reliably touch `lastPublish`. True for cascades of many short tools;
  false for a single long tool. Document the limitation honestly.

### Operator override unchanged

`HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` continues to override the
default. No code change at `autoWakeStuckThreshold()` (line 259).

## What we considered and rejected

### Rejected: inject a `LivenessProbe` from the API server into the org spawner

This was the previous iteration of this design. The reviewer flagged
it as wrong-layered and the call is right: stuck detection belongs in
the session layer, not the org layer. Plumbing WS / streaming-context
state into `api/pkg/org/infrastructure/runtime/helix/spawner.go` would
have entrenched the layering violation. Removing the org-layer
timeout entirely is both simpler and architecturally cleaner.

### Rejected: bump the auto-wake threshold only, don't touch the org layer

Treats the symptom. The decoy interactions still exist; they just sit
longer before the worker engages on them. Future operators reading the
SQL filter would find empty waiting rows piling up on healthy sessions
and wonder why. Fix the root cause at the org layer.

### Rejected: keep `ActivationTimeout` as-is, just longer (e.g. 4 hours)

A longer timeout still embeds the same wrong-layer assumption — "the
org layer knows when an activation is too old to be alive." Picking 4
hours instead of 5 minutes still false-positives for any task that
legitimately runs longer (training runs, large migrations, batch
processing). The runaway guard at 24 hours exists for resource safety,
not for liveness detection.

### Rejected: change the SQL filter to exclude decoys

You can't reliably exclude a decoy at the SQL layer — by construction
it looks exactly like a real stuck row. The only way to make the
filter ignore it is to never create it. Decision 1 does that.

## Patterns picked up

- Constants tunable via env vars use the read-once-per-call pattern at
  `auto_wake_stuck_interactions.go:259-279`. The runaway guard is not
  tunable; do not invent a new pattern, simply use a const.
- `SpawnerConfig` already separates concerns via distinct fields with
  individual defaults populated at the top of `Spawner` (lines
  140-150). Splitting `ActivationTimeout` into two fields follows
  the existing shape.
- Renames need a `grep` over the whole repo. Test wirings, dev wirings,
  documentation, and config-parsing code all reference the old name.

## Quick mitigation while the fix rolls out

Operators on affected instances can set
`HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS=180` (or higher) immediately
without redeploying. This is Decision 2 applied at runtime. Decoy
interactions will still appear (because Decision 1 is not deployed)
but the gate will not false-positive on them.
