# Out-of-order interaction: concurrent queue-drain race (2026-06-23)

Task: `spt_01kvsf30y4xxkz976hfhe4a3dc` ("Subscription Mode Should Default to Latest Opus")
Session: `ses_01kvsf30yr0x8f4s070n0engmp`  Zed thread: `2f358321-b4c0-434d-ab46-45b4c966930d`

## Symptom

User typed two messages while a turn was streaming. They were processed in the
wrong order; the older one is still streaming *above* the newer one which has
already completed, so live updates appear inserted in the middle of the
transcript ("not at the end"), confusing the reader.

## The two messages (three orderings, all disagree)

| Prompt | Submitted (prompt_id ts) | Interaction created | Zed msg id | State |
|---|---|---|---|---|
| A "switching from external agent…" | **07:04:43.930** (first) | 07:05:31.899601 | 512→534 (later) | **waiting / still streaming** |
| B "but then refreshing the page…" | **07:04:54.753** (second) | 07:05:31.899716 | 510 (earlier) | complete @ 07:06:01 |

- **Submission order:** A then B (≈11s apart).
- **Display order:** A then B (created_at / ULID tiebreak — A first).
- **Zed processing order:** B then A (B = msg 510, A = msg 512+). **Reversed.**

Both were typed while "oh it works now!" (`…39ghpv`, submitted 07:04:00, completed
07:05:31.784) was running, so both were **queued** (`prompt_history_entries`,
status pending, `queue_position=NULL`, `interrupt=f`). Note both interactions were
created within **115µs** of each other — they were drained simultaneously, not
one-after-another.

## Root cause: two drain goroutines, no per-session serialization

When the running turn completed, two different drain paths fired as goroutines
with nothing serializing them per session:

- `processAnyPendingPrompt` (readiness/idle path, sync.go:3780) — **no busy check** —
  called `GetAnyPendingPrompt`, which atomically claimed the FIFO-oldest pending
  prompt = **A** (`gevjx6bqi`).
- `processPromptQueue` (turn-complete path, sync.go:3041) — its busy check passed
  (A's interaction not yet committed) — called `GetNextPendingPrompt`, which
  skipped A (already `status=sending`) and claimed the only one left = **B**
  (`coofq4cfo`).

Proven by the API log — interleaved, same second:

```
07:05:31Z 3102 [QUEUE] Processing next non-interrupt prompt … "but then refreshing…" coofq4cfo   (processPromptQueue → B)
07:05:31Z 3154 [QUEUE] Processing pending prompt          … "switching from external…" gevjx6bqi (processAnyPendingPrompt → A)
07:05:31Z 3129 [QUEUE] Successfully dispatched …          coofq4cfo
07:05:31Z 3182 [QUEUE] Successfully processed pending …   gevjx6bqi
```

Both prompts were dispatched to Zed concurrently. Zed serialized them in arrival
order (non-deterministic w.r.t. submission) and happened to run B (510) before
A (512+). A is the long turn and is still streaming; B already finished.

The `processPromptQueue` busy-check (sync.go:3075 — "defer if last interaction is
`waiting`") cannot prevent this: (1) `processAnyPendingPrompt` has no busy check
at all, and (2) it's a TOCTOU — the first claim's interaction isn't committed
before the second goroutine reads state.

## Why the UI looks wrong

Display sorts by `created_at` (assigned at drain). A sorts first (submitted first)
so it sits on top — but Zed answered B first, so the **top** interaction (A) is the
one still streaming while the one **below** (B) is already complete. Hence "updates
inserted not at the end." This is a *consequence* of the dispatch reversal, not a
separate frontend bug — if A had been processed first (correct FIFO), it would have
completed first and B would stream at the bottom.

## Fix direction

Serialize queue draining per session so only one queued prompt is in flight at a
time, in strict FIFO order:

1. A per-session keyed mutex (e.g. keyed by `sessionID`) held across
   claim + `sendQueuedPromptToSession` (interaction create) in **both**
   `processPromptQueue` and `processAnyPendingPrompt`, so the second goroutine's
   busy-check runs only after the first interaction is committed `waiting` — and
   then defers.
2. Apply the same busy-check to `processAnyPendingPrompt` (currently missing).

Either alone is insufficient: (1) without the busy-check in the "any" path, the
two paths still claim different prompts; (2) the busy-check without a lock is the
TOCTOU that already failed here. Want both: one drain at a time, busy-defer if a
turn is live.

No mass-targeting; single-session correctness fix.

## Frontend dimension: the interrupt toggle never reached the backend

User recollection: pressed empty-Enter and/or clicked the queue lightning toggle
to make these interrupts. Evidence says the backend never saw it:

- `updateInterrupt` (usePromptHistory.ts:696) mutates **localStorage only**
  (`syncedToBackend:false`) and relies on a background sync to PATCH the backend.
- The backend recorded both prompts `interrupt=f` for their whole lifetime and
  dispatched both via the non-interrupt paths (logs: `interrupt=false`). No
  interrupt PATCH for `gevjx6bqi`/`coofq4cfo` appears in the logs at all.

So whatever was toggled in the UI lost the race to the queue drain (turn completed
07:05:31 and the backend drained the stale `interrupt=f` before the local→backend
sync pushed `interrupt=true`) — or never fired. Either way the escalation silently
didn't apply. This is a second consistency gap: interrupt state is localStorage-
first and syncs lazily, so an escalation made shortly before a turn ends can be
ignored.

Separately, empty-Enter promotion is itself an ordering hazard:
`handleKeyDown` (RobustPromptInput.tsx:944-969) promotes the **most-recent**
queued non-interrupt (`reduce` by max timestamp) to interrupt. With [A(older),
B(newer)] queued, one empty-Enter escalates **B**, dispatching it ahead of A —
directly producing B-before-A. Pressing twice then escalates A too, and now two
interrupts race through `processInterruptPrompt` (same concurrency hole).

Even had the toggle synced, interrupts hit `processInterruptPrompt`, which shares
the unserialized-drain race — so the backend per-session drain lock is required
regardless of interrupt vs queue.

## Implemented

Branch `fix/queue-drain-out-of-order-dispatch`.

- **Backend per-session drain lock** (`HelixAPIServer.lockPromptDrain`, a
  `sync.Map` of per-session `*sync.Mutex`). Acquired at the top of
  `processPromptQueue`, `processAnyPendingPrompt` and `processInterruptPrompt` and
  held across claim → [cancel] → send → `CreateInteraction`. The second drain's
  busy re-check in `sendQueuedPromptToSession` now observes the committed Waiting
  interaction and defers (queue) or cancels in order (interrupt). The busy-defer
  for the readiness path falls out of this — no separate blanket busy-check was
  added (that would wrongly defer interrupts meant to land mid-turn). Unit test:
  `TestLockPromptDrainSerializesPerSession`.
- **Frontend immediate interrupt persistence** (`usePromptHistory.updateInterrupt`):
  toggling interrupt now pushes that single entry to the backend immediately
  instead of waiting for the 100ms debounced batch sync, closing the window where
  a queue drain reads a stale `interrupt=f`.
- **Frontend empty-Enter FIFO escalation** (`RobustPromptInput.handleKeyDown`):
  empty-Enter now promotes the OLDEST queued non-interrupt message, not the
  newest, so repeated presses escalate in submission order instead of dispatching
  a newer message ahead of older ones. Test updated.
