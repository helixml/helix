# Requirements: Stop Auto-Wake From Cutting Short Long Tool-Call Sessions

## Background

Worker sessions (especially docs-writing or any work that runs slow shell
tools like `git push`, `npm install`, `gh pr view`, `find /`) are being
interrupted after ~2 minutes of apparent activity. The chat UI shows the
agent happily streaming thinking and tool calls, then mid-tool it gets
"interrupted" and the prompt is re-sent. Effectively this caps the
practical length of a worker session and breaks longer tasks.

Root cause analysis is captured in the user request that opened this
spec. Two compounding bugs combine to produce the symptom:

1. **The decoy interaction.** `api/pkg/org/domain/activation/queue.go`
   serialises spawns per-Worker, but only while the spawner is inside
   `pollUntilDone`. `pollUntilDone` exits when the `actCtx` fires, which
   is `cfg.ActivationTimeout = 5 * time.Minute`
   (`api/pkg/org/infrastructure/runtime/helix/spawner.go:147`) — **not**
   when the session actually completes. When the 5-min timer fires on a
   real-but-slow session, the Queue lane releases. The next pending
   trigger drains and starts a fresh interaction on the same session,
   even though the agent is still working on the prior turn. That new
   interaction lands with `state=waiting`, empty `response_message`,
   NULL `response_entries`. From the SQL filter's perspective it is a
   perfectly stuck row.

2. **The wake-up threshold is shorter than common tool durations.**
   `defaultAutoWakeStuckThreshold = 60 * time.Second`
   (`api/pkg/server/auto_wake_stuck_interactions.go:210`). The
   streaming-context gate at lines 416-426 skips wake-up only if
   `time.Since(sctx.lastPublish) < 60s`. `lastPublish` is updated only
   when an ACP `session/update` event arrives
   (`websocket_external_agent_sync.go:1333` / `:1359`). The agent emits
   `session/update` *around* tool calls, **not during them** — a
   90-second `git push` produces zero streamed events, so `lastPublish`
   decays past the 60s threshold and the gate fails by one second.

The "Incomplete interaction" banner the user sees is on the decoy row,
not on the interaction the agent is actually writing into. The agent
itself was never stuck.

## User Stories

### Story 1: Long docs-writing session completes without auto-wake interruption
**As a** worker operator running a docs-writing task
**I want** my session to keep running through multi-minute tool calls
(`git push`, `npm install`, large file writes)
**So that** the agent finishes the work I assigned instead of being
interrupted and re-prompted mid-stream.

**Acceptance:**
- A session whose tool calls produce no ACP `session/update` events for
  up to 5 minutes is NOT auto-woken.
- A session whose underlying activation hits the 5-min
  `ActivationTimeout` does NOT have a second interaction spawned on
  top of it while the agent is still alive.
- No "Incomplete interaction" / "↻ Retried" banner appears on a session
  whose only "stuckness" is a long-running tool call.

### Story 2: Genuinely stuck sessions are still detected
**As a** worker operator
**I want** auto-wake to still fire when the agent is genuinely silent
(claude-agent-acp wrapper buffered the tail of a turn — the bug
auto-wake exists to mitigate)
**So that** I don't lose the original behaviour that motivated the
auto-wake worker in the first place.

**Acceptance:**
- An interaction whose session has a live WebSocket but where neither
  `lastPublish` nor `session.Updated` has moved for ≥ the (new, longer)
  threshold is still woken.
- The two-retry cap (`autoWakeMaxRetries = 2`) and the error-state
  fall-through continue to apply.

### Story 3: Operator can tune wake aggressiveness without redeploy
**As a** Helix operator
**I want** to be able to push the wake threshold higher (or lower) at
runtime via env var
**So that** I can mitigate the issue in production while the proper fix
is rolling out.

**Acceptance:**
- `HELIX_AUTO_WAKE_STUCK_THRESHOLD_SECONDS` continues to override the
  default. (Already implemented; verify it survives the changes.)
- The default value is raised to a number that covers the realistic
  tool-call envelope (see Design for the chosen value and rationale).

## Out of Scope

- **Adding a heartbeat to claude-agent-acp / Zed.** That is the
  architecturally correct fix (a real liveness signal during tool
  execution) but requires protocol-level changes in
  agent-client-protocol / claude-agent-acp / Zed and is tracked
  upstream. This spec ships the Helix-side mitigations only.
- **Re-architecting the activation Queue.** We are not rewriting
  per-Worker serialisation. The change is: change *when* the lane is
  released, not how it is keyed.
- **Removing the auto-wake worker.** It still solves a real problem
  (claude-agent-acp's outbound buffer holding the tail of a turn). We
  are tightening its false-positive rate, not deleting it.

## Success Criteria

- Reproduce the failure mode pre-change: a session that runs a 90-120s
  `sleep` inside a shell tool reliably gets the wake-up banner today.
- Post-change: the same session completes without a banner and without
  a second interaction being created.
- Inner-Helix smoke test: register, create a spec task, ask the agent
  to run a slow command — observe completion without retry markers.
