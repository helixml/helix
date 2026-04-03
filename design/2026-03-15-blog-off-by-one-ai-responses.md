# The Off-by-One Bug That Made Our AI Responses Land in the Wrong Place

*How a flat map became a FIFO queue, and why GPUI's event model and ACP's cancel mechanism saved us*

---

At Helix, we run AI coding agents inside sandboxed desktop environments. Each agent lives in a Zed IDE instance, connected to our Go API server over a persistent WebSocket. When the agent responds to a message, streaming tokens flow from Zed → Go → your browser in real time.

Recently we encountered a maddening bug: responses were appearing in the **wrong conversation cards**. You'd send a message, the agent would reply — but the reply would visibly land inside the *previous* message's card. Or worse, responses from two different messages would get interleaved across the wrong cards entirely. Users described it as "off-by-one or off-by-several."

This is the story of finding it, fixing it, and the surprisingly deep reasoning about event ordering we had to do along the way.

---

## The System

To understand the bug, you need to understand the architecture.

Each Helix "session" is a conversation with an AI agent. The conversation is stored as a series of **interactions** in our Postgres database — alternating user messages and assistant responses. When the agent responds, it streams tokens via a WebSocket event called `message_added`, and signals completion with `message_completed`.

On the Go side, we maintain an in-memory mapping:

```
sessionToWaitingInteraction[sessionID] → interactionID
```

This tells the streaming handler: "when tokens arrive for this session, write them into *this* interaction." The mapping is set when a new message is created, and cleared when `message_completed` arrives.

The problem is that multiple things can set this mapping.

---

## The Race Condition

In our system, users can trigger new messages in several ways while the agent is still responding:

1. **Typing in the Helix sidebar** and pressing Enter (queue or interrupt mode)
2. **Clicking Approve/Reject** on a spec task review
3. **Typing directly in Zed's text box** and pressing Enter

For the Helix-initiated paths, our code creates a new DB interaction and then writes its ID into the mapping — before sending the message to Zed. This is correct: it ensures the mapping exists before any response tokens arrive.

But consider what happens if interaction I_A is still streaming when you send a new message that creates interaction I_B:

```
sessionToWaitingInteraction["ses_xyz"] = "int_A"    ← I_A streaming
                                                      ← ... tokens flowing ...
sendMessageToSpecTaskAgent() called:
  create I_B in DB
  sessionToWaitingInteraction["ses_xyz"] = "int_B"  ← OVERWRITES!
                                                      ← ... more I_A tokens arrive ...
                                                      ← they now route to int_B ❌
message_completed(I_A):
  find "most recent waiting interaction" → finds I_B (newest) ❌
  marks I_B as complete before it even has content
```

The result: I_A's remaining streaming content lands in I_B's card. I_B gets marked complete immediately. The user sees a response in the wrong card, and the next actual response has nowhere to go.

---

## The Fix: From Map to Queue

The fix is conceptually simple. Instead of a flat map that gets overwritten, use a **FIFO queue per session**:

```go
// Before
sessionToWaitingInteraction map[string]string

// After
sessionToWaitingInteraction map[string][]string
```

Writers append to the back. The streaming context peeks the front. `message_completed` pops the front.

```
I_A streaming:   queue = ["int_A"]
sendMessage():   queue = ["int_A", "int_B"]    ← append, not overwrite
                 streaming tokens for I_A → peek queue[0] = "int_A" ✓
message_completed(I_A):
                 queue = ["int_B"]              ← pop
I_B streaming:   peek queue[0] = "int_B" ✓
message_completed(I_B):
                 queue = []                     ← pop
```

Every token goes to the right card. The mapping is never lost.

We added one fallback: if the API server restarts mid-session, the in-memory queue is empty. In that case, `handleMessageCompleted` falls back to a DB scan for the most recent waiting interaction — good enough for the restart scenario, since the in-flight session's state is already disrupted.

---

## But Does the Ordering Actually Hold?

Fixing the routing was the easy part. The harder question: **does `message_completed(I_A)` always arrive at Helix before any tokens from I_B?**

If not, we'd have a different problem: I_B's tokens arriving while `queue[0]` is still I_A, routing them to the wrong interaction.

We traced through every path a new message can arrive from:

**User types in Zed text box, presses Enter mid-stream**

Zed's event subscription (`ensure_thread_subscription` in `thread_service.rs`) runs on GPUI's single foreground thread. All `AcpThreadEvent` variants — `NewEntry`, `EntryUpdated`, `Stopped` — are dispatched through this one callback in order. ACP cannot append a new user entry to the thread without first completing the `Stopped` event for the current turn. So the event sequence is always:

```
EntryUpdated(I_A) × N  →  Stopped(I_A)  →  NewEntry(user, I_B)  →  NewEntry(assistant, I_B)  →  ...
```

`Stopped(I_A)` → `message_completed(I_A)` → I_A popped from queue. By the time `NewEntry(user, I_B)` fires and Helix creates I_B's interaction, the queue is empty. I_B is appended and immediately at position 0.

**User sends from Helix, interrupt mode**

Helix pre-creates I_B and appends to queue before sending. Queue = `["int_A", "int_B"]`. Zed receives the follow-up while I_A's turn is still active in ACP. This is the path that warranted the most scrutiny.

In Zed, every message send goes through `AcpThread::run_turn()`. When the handler loop calls `send(I_B)`, `run_turn()` immediately *cancels* the active I_A turn before starting I_B:

```
handler loop: send(I_B)
  → run_turn() calls self.cancel()
    → sets suppress_abort_err = true
    → sends CancelNotification to ACP
    → ACP aborts I_A's prompt
    → abort error arrives back → converted to PromptResponse { Cancelled }
    → run_turn() emits AcpThreadEvent::Stopped   ← this is the key
    → subscription sends MessageCompleted to Helix
    → Helix: message_completed(I_A) → pops I_A → queue = ["int_B"]
  → I_B's send begins (cancel_task.await completed first)
  → I_B's tokens arrive → queue[0] = I_B ✓
```

The abort-to-`Cancelled` conversion is deliberate: the ACP connection sets `suppress_abort_err = true` before sending the cancel notification, so the inevitable abort error from ACP is converted into a clean `PromptResponse { stop_reason: Cancelled }` rather than an error. `run_turn()` always emits `Stopped` on the success path. No special-casing needed.

One might worry: what if the interrupt arrives before `run_turn()` has set `running_turn`, causing `cancel()` to return silently? In practice this window is zero — `running_turn` is set synchronously during the initial `entity.update()` call, before any `.await`. An interrupt from Helix can only arrive after the WebSocket message has been delivered to Zed, which is after that synchronous setup has already run.

**Queue mode (Helix sidebar, Enter)**

Queue-mode messages are only dispatched when the session is idle — `lastInteraction.State != Waiting`. Since interaction state is written to the DB immediately on creation and doesn't change until `message_completed`, the idle check is reliable. No mid-stream dispatch for queue mode.

**The guarantees in all cases come from three independent layers:**

1. **GPUI's foreground thread** serializes all `entity.update()` calls — no true concurrency, cooperative interleaving only
2. **ACP's `run_turn()` cancel mechanism** — a second `send()` always cancels the first and always produces a `Stopped` event through the abort-to-Cancelled conversion
3. **TCP/WebSocket ordering** — preserves event order end-to-end between Zed and Go

The FIFO queue is the *backend* routing mechanism. The layers above are why the invariant "message_completed(I_A) before any I_B tokens" holds for every path.

---

## The Echo Problem (Bug 1)

While we were in here, we fixed a related bug.

For Helix-initiated messages, we pre-create the interaction before sending to Zed. Zed then echoes the message back as `message_added(role=user)`. Without special handling, `handleMessageAdded` would create a *second* interaction for the same message, overwrite the queue mapping with the new interaction's ID, and lose track of the pre-created one.

The fix: before creating a new interaction for a `role=user` event, peek at `queue[0]`. If a pre-created interaction already exists, this event is an echo — discard it, keep the existing mapping.

```go
apiServer.contextMappingsMutex.RLock()
var existingInteractionID string
if q := apiServer.sessionToWaitingInteraction[helixSessionID]; len(q) > 0 {
    existingInteractionID = q[0]
}
apiServer.contextMappingsMutex.RUnlock()

if existingInteractionID != "" {
    // Echo of a Helix-initiated message — reuse pre-created interaction, do nothing
} else {
    // Genuine user message from Zed — create new interaction, append to queue
}
```

---

## The GORM Bool Default (Bug 2)

Unrelated, but fixed at the same time: GORM treats Go's zero values as "not set" and skips them in INSERT statements. For a `bool` field, `false` is the zero value — so an INSERT with `interrupt = false` would silently apply the column's DEFAULT instead.

Our `prompt_history` table had `DEFAULT TRUE` on the `interrupt` column (correct for most entries). But queue-mode messages have `interrupt = false`. GORM would skip writing the false, the column default of TRUE would be applied, and queue-mode prompts would be treated as interrupt-mode prompts — sent immediately even if the session was busy.

Fix: change the GORM tag to `default:false` and add a startup `ALTER TABLE` to fix the column default in existing databases.

```go
// Before
Interrupt bool `gorm:"column:interrupt;not null;default:true"`

// After
Interrupt bool `gorm:"column:interrupt;not null;default:false"`
```

This is a known GORM footgun. If you're using GORM and have bool columns with non-zero defaults, verify your INSERT behaviour.

---

## What We Learned

A few things worth carrying forward:

**Don't use a flat map when order matters.** If multiple producers can update a shared routing table, and the order those updates happen matters for correctness, you need a queue — not a map. The flat map worked fine in the common case (one message at a time, clean handoffs), which is exactly why the bug was hard to reproduce.

**Don't rely on UI timing for backend correctness.** An early version of our reasoning included "users can only click Approve after `message_completed` is received." That's true — today, with that UI. But it's not a backend guarantee. The FIFO queue makes the system correct regardless of when any message is triggered.

**Understand your event model before assuming ordering.** The key insight that made us confident in the fix was understanding GPUI's single-foreground-thread event dispatch. Events from the same ACP thread entity are always ordered. Without that knowledge, we might have added defensive delays or polling — which would have been wrong and slow.

**Read the cancel path before assuming it's a problem.** The interrupt-mode race looked dangerous: two tasks calling `send()` on the same entity while one is mid-response. The naive fear was that ACP might silently drop the second call. The actual behavior — `run_turn()` cancels the first turn, the cancel notification triggers a `Stopped` event via the abort-to-Cancelled conversion, the FIFO queue is updated correctly — is exactly right. We only knew this by reading the code, not by reasoning about it from the outside.

**One queue, three guarantees.** The FIFO queue provides routing correctness. GPUI's cooperative scheduler provides entity-update serialization. ACP's `run_turn()` cancel mechanism ensures cancelled turns always produce `Stopped`. TCP/WebSocket preserves event order end-to-end. Each layer is independent — and each is necessary. The queue alone is insufficient without the ordering guarantees. The ordering guarantees are invisible without the queue.

---

The fix was about 150 lines across 7 files, mostly mechanical type changes from `string` to `[]string`. The analysis took longer. The 46 unit tests all pass.

*Helix is an AI-native development environment. We build on a forked Zed IDE, with a Go API server, a Rust WebSocket sync layer, and a React frontend. If you're interested in the architecture, our design docs are in `design/` in the repository.*
