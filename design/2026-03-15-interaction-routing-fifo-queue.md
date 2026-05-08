# Interaction Routing: The FIFO Queue Fix

## Date
2026-03-15

## Status
**Implemented** — `feature/001546-we-recently`, commit `11209d5b9`

---

## Problem Statement

Users reported that AI responses appeared in the wrong Helix session interaction cards — responses from one exchange landing visually inside a different exchange's card. The symptom was described as "off-by-one or off-by-several" — sometimes a full response's worth of streaming content ended up in the preceding card, and the card that should have received it stayed empty.

Separately, `handleMessageCompleted` was marking the wrong interaction as complete when a new message arrived while a previous response was still streaming.

---

## Root Cause: `sessionToWaitingInteraction` Was a Flat Map

```go
// BEFORE: flat map — any write overwrites the current mapping
sessionToWaitingInteraction map[string]string // SessionID → InteractionID
```

The map was written by three callers:
- `sendMessageToSpecTaskAgent` — approve/reject/review comment flows
- `sendQueuedPromptToSession` — queue-mode and interrupt-mode prompts from Helix
- `handleMessageAdded(role=user)` — user typing directly in Zed

If `sendMessageToSpecTaskAgent` was called while interaction I_A was still streaming (e.g. the user clicked Approve before the agent finished), it would:
1. Create I_B in the DB
2. **Overwrite** `sessionToWaitingInteraction[session]` with I_B's ID

From that point, all subsequent `message_added(role=assistant)` events for I_A — which use `getOrCreateStreamingContext` to look up the mapping — would find I_B's ID and route I_A's content into I_B's card. Meanwhile, when `message_completed` arrived for I_A, `handleMessageCompleted` scanned the DB for the "most recent waiting interaction" and found I_B (the newest), marking it complete even though I_B's response hadn't started yet.

---

## Fix: FIFO Queue Per Session

```go
// AFTER: FIFO queue — writers append, readers peek/pop
sessionToWaitingInteraction map[string][]string // SessionID → []InteractionID (queue)
```

**Writers append to the back:**
```go
s.sessionToWaitingInteraction[sessionID] = append(
    s.sessionToWaitingInteraction[sessionID], createdInteraction.ID)
```

**Streaming context peeks the front:**
```go
if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
    expectedInteractionID = q[0]
}
```

**`handleMessageCompleted` pops the front:**
```go
apiServer.contextMappingsMutex.Lock()
if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
    targetInteractionID = q[0]
    if len(q) == 1 {
        delete(apiServer.sessionToWaitingInteraction, helixSessionID)
    } else {
        apiServer.sessionToWaitingInteraction[helixSessionID] = q[1:]
    }
}
apiServer.contextMappingsMutex.Unlock()

// Fallback: DB scan for waiting interaction (handles API restart, queue lost)
if targetInteractionID == "" {
    for i := len(interactions) - 1; i >= 0; i-- {
        if interactions[i].State == types.InteractionStateWaiting {
            targetInteractionID = interactions[i].ID
            break
        }
    }
}
```

With the queue, if `sendMessageToSpecTaskAgent` is called while I_A is streaming, the queue becomes `[I_A, I_B]`. Streaming events for I_A still peek `queue[0] = I_A` ✓. When `message_completed` arrives for I_A, it pops I_A → `[I_B]`. I_B's streaming then correctly routes to `queue[0] = I_B` ✓.

---

## Who Creates Interactions and When

This is critical for understanding the ordering guarantees.

### Helix-originated messages

All three Helix-initiated paths **pre-create the interaction in the DB before sending to Zed**:

| Path | Function | Creates interaction |
|------|----------|-------------------|
| Approve/Reject/Review comment | `sendMessageToSpecTaskAgent` | Yes, immediately |
| Queue-mode prompt (Helix text box, Enter) | `sendQueuedPromptToSession` | Yes, immediately |
| Interrupt-mode prompt (Helix text box, Ctrl+Enter) | `sendQueuedPromptToSession` | Yes, immediately |

Zed echoes the sent message back as `message_added(role=user)`. The **Bug 1 fix** in `handleMessageAdded` peeks the queue front: if a pre-created interaction exists, the echo is silently discarded (no duplicate interaction, no queue overwrite).

### Zed-originated messages

When the user types directly in the Zed text box and presses Enter, **Helix has no prior knowledge**. The interaction is created only when the `message_added(role=user)` echo arrives from Zed.

---

## Ordering Guarantees

### The backend guarantee: GPUI event dispatch

The Zed subscription in `thread_service.rs` (`ensure_thread_subscription`) runs on **GPUI's single foreground thread**. All `AcpThreadEvent` variants — `NewEntry`, `EntryUpdated`, `Stopped` — are dispatched through this one callback in order. ACP cannot add a new user entry to the thread without first completing the `Stopped` event for the current turn, because both are entity-level operations serialized on the foreground thread.

This means Helix always receives WebSocket events in this order — regardless of which path triggered the new message:

```
EntryUpdated(I_A) × N  →  Stopped(I_A)  →  NewEntry(user, I_B)  →  NewEntry(assistant, I_B)  →  EntryUpdated(I_B) × K  →  Stopped(I_B)
       ↓                        ↓                    ↓                          ↓
message_added(I_A) × N   message_completed(I_A)   message_added(role=user,   message_added(role=assistant, I_B) × K
                                                   I_B) [creates or echoes]
```

TCP/WebSocket preserves this order end-to-end. Helix's WebSocket reader goroutine processes messages sequentially.

### Per-path ordering analysis

| Path | Queue state when I_B arrives | After `message_completed(I_A)` | I_B assistant events route to |
|------|------------------------------|-------------------------------|-------------------------------|
| User types in Zed mid-stream | `[]` (I_A already popped when `NewEntry(user,I_B)` fires) | `[I_B]` (just appended) | `queue[0]` = I_B ✓ |
| Helix queue mode | `[]` (only sent when session is idle) | `[I_B]` | `queue[0]` = I_B ✓ |
| Helix interrupt mode | `[I_A, I_B]` (pre-created before send) → after pop = `[I_B]` | `[I_B]` | `queue[0]` = I_B ✓ |
| Approve/Reject/Review | same as interrupt | `[I_B]` | `queue[0]` = I_B ✓ |

### Why "Stopped fires before NewEntry(user)" is guaranteed

For the user-types-in-Zed path, the key question is: can `NewEntry(user, I_B)` fire before `Stopped(I_A)`?

No. Whether ACP cancels I_A immediately on receiving the new message or queues I_B internally, both cases result in `Stopped(I_A)` being dispatched on the GPUI foreground thread before `NewEntry(user, I_B)` is dispatched, because ACP cannot append a new user entry to the thread while the current turn is still marked as running. The subscription sees events in the order ACP commits them to the thread entity.

This means for the Zed-typing path, by the time `message_added(role=user, I_B)` arrives at Helix, `message_completed(I_A)` has already been processed and I_A has been popped. The queue is empty when I_B is appended, so `queue[0] = I_B` immediately.

### Queue mode: the idle check

`processPendingPromptsForIdleSessions` checks `lastInteraction.State == InteractionStateWaiting` against the DB to determine if the session is busy. This is reliable because:
- `CreateInteraction` writes `state=Waiting` immediately (no throttling on create)
- State stays `Waiting` until `handleMessageCompleted` writes `state=Complete`
- Queue-mode messages are only dispatched when the state is NOT `Waiting`, so no concurrent queue message can be in flight

### Interrupt mode: ACP cancel mechanism resolves the race

When `processInterruptPrompt` sends a follow-up while the initial thread creation's detached task is still awaiting I_A's response, a second `AcpThread::send()` call fires while the first is still in progress. This was the one area of concern. Code analysis of `acp_thread.rs` shows it is safe.

**GPUI entity updates are sequentially interleaved, not truly concurrent.** GPUI runs on a single foreground thread. Async tasks cooperatively yield at `.await` points — they don't run in parallel. The two `entity.update(cx, |t, cx| t.send(...))` calls cannot overlap; the second can only run while the first is suspended at `.await`.

**`run_turn()` has a built-in cancel-then-restart mechanism.** Every call to `AcpThread::send()` goes through `run_turn()`, which does:

```rust
fn run_turn(&mut self, cx, f) -> BoxFuture<...> {
    let cancel_task = self.cancel(cx);   // cancels any existing running turn
    self.turn_id += 1;
    self.running_turn = Some(RunningTurn {
        send_task: cx.spawn(async move |this, cx| {
            cancel_task.await;           // waits for old turn to fully cancel
            tx.send(f(this, cx).await).ok();
        }),
    });
    Box::pin(async move { rx.await.ok() })
}
```

When the handler loop calls `send(I_B)`, `run_turn()` calls `self.cancel(cx)` which takes ownership of turn I_A's `running_turn` and sends a `CancelNotification` to ACP. The new turn I_B does not start until the cancellation completes.

**Cancel is converted to a `Stopped` event, not an error.** The real ACP connection sets `suppress_abort_err = true` before sending the cancel notification. When ACP responds with an abort error, the connection converts it to a successful `PromptResponse` with `StopReason::Cancelled`. `run_turn()` then takes the `Ok(Cancelled)` path, which **always** emits `AcpThreadEvent::Stopped`. The subscription in `thread_service.rs` catches this and sends `MessageCompleted` to Helix.

The full cancel chain:

```
handler loop: send(I_B)
  → run_turn(): self.cancel()
    → connection.cancel(): sets suppress_abort_err=true, sends CancelNotification to ACP
    → ACP aborts I_A's prompt() call
    → connection: abort error detected, suppress_abort_err=true
      → converts to PromptResponse { stop_reason: Cancelled }
    → run_turn() Ok(Cancelled) path: cx.emit(AcpThreadEvent::Stopped)
    → subscription: MessageCompleted sent to Helix
    → Helix: message_completed(I_A) → pops I_A from queue → [I_B]
  → cancel_task.await completes
  → I_B's send begins
  → I_B's tokens arrive → queue[0] = I_B ✓
```

**The `running_turn` is always set before an interrupt can arrive.** The theoretical edge case where `cancel()` returns silently (when `running_turn` is `None`) cannot occur here. `running_turn` is set *synchronously* inside `run_turn()` during the initial `entity.update()` call, before any `.await`. The handler loop can only receive an interrupt after Helix has delivered the initial WebSocket message to Zed, which happens after the detached task has already called `send()` and set `running_turn`. The window is zero.

**ACP's own test covers this scenario.** `test_follow_up_message_during_generation_does_not_clear_turn` in `acp_thread.rs` explicitly tests that a second `send()` during an active turn sets `running_turn` to turn 2, and that turn 1 completing does not clear it. The cancel-to-Stopped path is exercised by the stub connection's abort-error handling.

The interrupt mode path is safe. No additional serialization is needed in Helix.

---

## Files Changed

| File | Change |
|------|--------|
| `api/pkg/controller/external_agent_mappings.go` | Type `*map[string]string` → `*map[string][]string`; `SetWaitingInteraction` uses append |
| `api/pkg/server/server.go` | Field and initialization updated |
| `api/pkg/server/websocket_external_agent_sync.go` | 7 sites: append for writes, peek for reads, pop-with-fallback in `handleMessageCompleted` |
| `api/pkg/server/spec_task_design_review_handlers.go` | `sendMessageToSpecTaskAgent` appends; comment linking uses queue back (most recently added) |
| `api/pkg/server/test_helpers.go` | Updated for new type |
| `api/pkg/server/prompt_history_handlers_test.go` | Updated for new type |
| `api/pkg/server/websocket_external_agent_sync_test.go` | Updated: 46 tests, all pass |

---

## Related Bugs Fixed in This Iteration

### Bug 1: Zed echo creating duplicate interactions (also fixed)

When Helix pre-creates an interaction and Zed echoes back `message_added(role=user)`, `handleMessageAdded` previously created a second interaction and overwrote the queue mapping. Now it peeks `queue[0]`: if a pre-created interaction exists, it reuses it and skips creation.

### Bug 2: GORM `bool` zero-value causing `interrupt=false` to save as `true`

GORM treats `false` as a zero value and omits it from INSERT statements, causing the column DEFAULT (which was `TRUE`) to be applied instead. Fixed by:
- Changing the GORM tag from `default:true` to `default:false` on `PromptHistoryEntry.Interrupt`
- Adding a startup `ALTER TABLE` migration to change the column default in existing databases
