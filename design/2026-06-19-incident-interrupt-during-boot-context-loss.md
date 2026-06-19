# Incident: interrupt during agent boot drops the initial message → agent runs with no context

**Date:** 2026-06-19
**Severity:** High (silent context loss — agent answers with no task context, no error surfaced)
**Status:** Root-caused, FIXED (three guards), content-verified against live Zed on meta.helix.ml (2026-06-19). The first PR (#2660) was **incomplete** — see "Follow-up: the first fix missed a concurrent dispatch path" at the end.
**Affected:** spec task `spt_01kvfq6m8a07gywpj6jmadyrb8`, planning session
`ses_01kvfq6m92snjwagqegd4e1zzk` (`agent_type=zed_external`, `code_agent_runtime=claude_code`).
**Related:** `design/2026-06-19-acp-v2-and-websocket-sync-rewrite-strategy.md`
(this is that correlation fragility manifesting acutely at boot), #2642, #2643.

---

## Summary

A user created a spec task, then — while the Zed/Claude agent was still booting
(~56s) — sent a quick correction ("Obviously I meant anger and Opus") as an
**interrupt**. The agent ended up running in a fresh ACP thread containing *only*
the correction, with the initial spec-writing prompt absent. The agent had no
task context. No error was surfaced to the user.

Two things combined to lose the context:

1. **The interrupt cancelled the *initial* turn.** Because boot was slow, the
   initial message had been delivered to Zed only ~13s earlier and was the
   active turn. The interrupt cancelled it before the agent processed it.
2. **The interrupt was dispatched with an empty `acp_thread_id`.** At send time
   the session had no confirmed Zed thread yet (the `thread_created` events
   arrived 2–3s *later*), so Zed spun the correction into a **brand-new thread**,
   divorced from the initial message's thread. Cancelling a turn normally keeps
   the user message in history — but only within the *same* thread. The new
   thread had no history at all.

---

## Timeline (UTC, from `helix-api-1` logs + DB)

| Time | Event |
|---|---|
| 10:35:37.107 | Spec task created; planning session `…92sn` claimed. |
| 10:35:37.126 | **I1** (initial PLANNING prompt) created as interaction `int_01kvfq6m968xz0a0szadvw8wya`, state `waiting`. `request_id=req_01kvfq6m968xz0a0szad2y26t9` registered → session (`external_agent_handlers.go:199`). |
| 10:35:39 | `StartDesktop` → dev container `ubuntu-external-…92sn` created. Agent booting. |
| 10:35:47 | `thread_created acp_thread_id=7ced99b8…` (Zed boot/workspace thread — noise). |
| 10:36:04.866 | User submits correction "Obviously I meant anger and Opus" as a `prompt_history_entry` with **`interrupt=true`**. |
| 10:36:05–10:36:28 | Repeatedly deferred: *"Session is busy (interaction waiting), queue prompts will be processed after message_completed"* (`prompt_history_handlers.go:265`). Agent not yet WS-connected; I1 still `waiting`. |
| 10:36:15 | `thread_created acp_thread_id=c58a8230…` (second boot thread — noise). |
| 10:36:28 | `pickupWaitingInteraction` queues I1's initial `chat_message` into the readiness buffer (`websocket_external_agent_sync.go:578`, `:2332`), `queue_size=1`, awaiting `agent_ready`. |
| **10:36:33** | `agent_ready` → **I1 finally delivered to Zed** ("Sent queued message type=chat_message", `:2273`). I1's first turn starts. **Boot delay: 56s.** |
| 10:36:34 | Queue-mode tries the correction (`processAnyPendingPrompt`) → **rejected**: *"session became busy (interaction waiting)"* (`:3112`); reverted to pending. |
| **10:36:46** | Interrupt branch fires (`prompt_history_handlers.go:259`): `processInterruptPrompt` → `cancelCurrentTurnIfActive` **cancels I1's active turn** (`request_id=req_01kvfq6m968…`, `:356`) → Zed `turn_cancelled status=noop`. |
| 10:36:46 | `sendQueuedPromptToSession` creates **I2** (`int_01kvfq8rdk7w1xsd02gx7vmqg5`) for the correction and sends it **with `acp_thread_id=` (EMPTY)** (`:3297`). → Zed creates a new thread. |
| 10:36:48 | `thread_created acp_thread_id=4186854b…` (the correction's new thread). |
| 10:36:49 | `thread_created acp_thread_id=b9c083ff…` (eventually the session's stored `zed_thread_id`). |
| 10:36:51 | Streaming-context chaos: *"Auto-completed cancelled interaction (interrupt arrived before message_completed)"* fires for **both** I1 and I2; repeated "Interaction transition detected! Resetting streaming context" ping-pong between the two (`:1513`, `:1576`). |
| 10:41:42 | A 400KB response streams under `context_id=4186854b` mapped to interaction `int_01kvfq6m968` (I1) — the late, cross-wired response. |

Net user-visible result: the agent's live thread (`4186854b`) contained only
"Obviously I meant anger and Opus". I1's spec-writing prompt was cancelled and
not present in that thread. Both interactions later show `complete` in the DB,
hiding the loss.

---

## Root cause

The follow-up was classified as an **interrupt** and dispatched **during the
boot race**, where the normal precondition of an interrupt — "there is an
established turn, in an established thread, that the user wants to redirect" —
did not hold. Specifically:

- The "active turn" the interrupt cancelled was the session's **initial** turn,
  delivered only 13s earlier and not yet processed. Cancelling it discarded the
  foundational context rather than refining it.
- `sendQueuedPromptToSession` sent the interrupt while `session.Metadata.ZedThreadID`
  was still empty (no `thread_created` yet), so `acpThreadID` resolved to `nil`
  → **Zed created a new thread** for the correction, with no shared history.

This is the `request_id`/`acp_thread_id` correlation fragility from the
2026-06-19 strategy doc, surfacing at its sharpest moment: boot, when no thread
identity exists yet and multiple `thread_created`s are racing in.

### Why existing guards didn't catch it
- The busy-check correctly *deferred* the correction as a queue-mode message
  (10:36:05–10:36:34). But once I1 was finally delivered and went `waiting`, the
  queue-mode dispatch was rejected as "busy", and the prompt's `interrupt=true`
  flag then routed it to `processInterruptPrompt`, which is *designed* to bypass
  the busy guard and cancel the active turn. The interrupt path has no notion of
  "the active turn is the initial message and the thread isn't established yet."
- `cancelCurrentTurnIfActive` / `sendQueuedPromptToSession` never check whether a
  confirmed `ZedThreadID` exists before dispatching — so an empty-thread send
  (new-thread fork) is possible at any time during boot.

---

## A second path, found while testing

While live-testing the fix we reproduced the *same* user-visible symptom through
a **different** code path, and fixed both. The unifying principle (per the
discussion that produced this fix): **first-message primacy must be enforced** —
the genuine first message must land and establish the thread before anything
else can preempt it. It was broken in two places:

- **Path 1 — queued interrupt (the documented incident):** `processInterruptPrompt`
  cancels the just-started initial turn and dispatches with an empty
  `acp_thread_id`, forking a new thread.
- **Path 2 — `pickupWaitingInteraction`:** when an initial + a follow-up both sit
  `waiting` at reconnect (e.g. a follow-up sent via the direct
  `/sessions/{id}/messages` path during boot, persisted as a waiting interaction),
  pickup delivered the **most-recent** waiting interaction and orphaned the
  initial — agent ran with only the follow-up.

## Fix (implemented + validated)

**Path 1 — thread-establishment barrier** (`prompt_history_handlers.go`, busy-interrupt
branch of `processPendingPromptsForIdleSessions`): if the session is busy but
`session.Metadata.ZedThreadID == ""`, *defer* the interrupt (leave it pending, do
not cancel, do not dispatch). The next poll retries; once `thread_created` sets
`ZedThreadID` the interrupt fires into the SAME, established thread. Placed at the
call site (not inside `processInterruptPrompt`) so the prompt is never claimed and
the idle-first-message path is untouched.

**Path 2 — pickup oldest-first** (`websocket_external_agent_sync.go`,
`pickupWaitingInteraction`): deliver the **oldest** waiting interaction (FIFO, the
genuine first / thread-creating message) instead of the newest. Any trailing
waiting interaction is delivered on the next turn (auto-wake / reconnect).

### End-to-end validation (live Zed, meta.helix.ml, 2026-06-19)
- **Path 2:** fresh spec task; fired a direct-message interrupt during boot.
  Result: the **initial** PLANNING prompt landed first and completed with a 67KB
  response; the follow-up did NOT preempt it. (Pre-fix repro: initial orphaned
  `waiting`/empty, agent answered only the follow-up.)
- **Path 1:** fresh spec task; queued interrupt + drove `/prompt-history/sync`
  through the boot window. Result: `⏸️ Busy but thread not established yet`
  deferred the interrupt **3×** while `ZedThreadID` was empty; after
  `thread_created` (`0864db66`) the interrupt delivered into that same thread;
  BOTH the initial (26KB) and the interrupt (18KB) completed. No thread fork, no
  context loss.
- Unit: `TestPromptHistoryHandlersSuite` + `TestWebSocketSyncSuite` green.

### Follow-up (not in this fix)
- The Zed e2e harness Phase 8 (interrupt) should grow a boot-window variant so
  this is covered in CI.
- Trailing waiting-interactions in the rare direct-message-during-boot case drain
  via auto-wake (~30s) rather than immediately; acceptable for the point fix.
- All of this is the `request_id`/thread correlation fragility from the strategy
  doc; the eventual v2 state-machine refactor subsumes both guards.

---

## Forensic notes / loose ends
- Three+ thread IDs appeared for one session (`7ced99b8`, `c58a8230`, `4186854b`,
  `b9c083ff`); the first two look like Zed boot/workspace threads (noise), but the
  multiplicity is what let the correction's empty-thread send go unnoticed.
- The cross-wired 400KB response at 10:41 (response for I1 streaming under the
  correction's thread `4186854b`) is the same correlation tangle and is worth a
  follow-up once the point fix lands.

---

## Follow-up: the first fix (#2660) missed a concurrent dispatch path

After #2660 merged, the original reproducer (move task to backlog → re-plan from
scratch → quick correction during boot) **failed identically**. Live trace
(session `ses_01kvfv3x…`, 11:44):

- The poller-side barrier (path 1) **did** fire — it deferred the interrupt-path
  dispatch 3×. But at **11:44:37** the correction was *also* picked up by a
  **different** path — `processAnyPendingPrompt → sendQueuedPromptToSession` —
  which is **exempt from the busy-defer for interrupt prompts**. It dispatched the
  correction with an **empty `acp_thread_id`** at the same moment the initial went
  out (also empty-thread).
- Two empty-thread sends → Zed forked **two** threads: initial's spec work →
  `ca419c1b`; correction → `b60a34c8`. Session bound to `b60a34c8`; the agent's
  correction response read verbatim *"a previous conversation context that I don't
  have."* Same symptom.

**Lesson:** guarding the poller decision site was too narrow — multiple dispatch
paths funnel into `sendQueuedPromptToSession`, and that is the real chokepoint.
Also: the #2660 "validation" checked message *completion + length*, not whether
the agent **retained context** — so it passed while the bug was live. Validate on
response content, not shape.

### The completing guard (chokepoint)

`sendQueuedPromptToSession` (`websocket_external_agent_sync.go`): the interrupt's
exemption from the busy-defer is only safe **once the thread exists**. Gate it:

```go
threadNotEstablished := session.Metadata.ZedThreadID == ""
if !prompt.Interrupt || threadNotEstablished {
    // busy-defer: if the newest interaction is a different Waiting one
    // (the initial, still creating the thread), return a retryable
    // "deferring" error instead of sending a second empty-thread message.
}
```

Until `ZedThreadID` is set, *every* prompt (interrupt included) respects the
busy-defer, so only the genuine first message is ever sent empty-thread. Once the
thread exists the interrupt is exempt again and fires into the SAME thread.

### Content-verified validation (live Zed, 2026-06-19)
Fresh spec task, initial = "WidgetSync … over **Bluetooth**", interrupt during
boot = "Actually … over **WiFi**". Result: session bound to **one** thread
(`04101764`); **zero** empty-thread correction sends; the agent's correction
response: *"The user wants to change the sync mechanism from Bluetooth to WiFi.
Let me now write the spec files for WidgetSync that syncs widgets between devices
over WiFi…"* — i.e. it had the initial feature **and** applied the correction. No
"context I don't have". This is the test that #2660's should have been.
