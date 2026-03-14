# Thread Subscription Architecture & Streaming Content Bug

## System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Helix Go API Server                          │
│                                                                     │
│  WebSocket ←──── message_added (streaming content)                  │
│  Handler   ←──── message_completed (turn finished)                  │
│            ←──── thread_created / thread_title_changed / etc.       │
│                                                                     │
│  Accumulator: same message_id = overwrite, diff message_id = append │
│  DB throttle: 200ms, Frontend publish throttle: 50ms                │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ WebSocket
                           │
┌──────────────────────────▼──────────────────────────────────────────┐
│                     Zed (Rust, in sandbox)                           │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │              thread_service.rs — Entry Point                │    │
│  │                                                             │    │
│  │  setup_thread_handler()                                     │    │
│  │    ├── callback_rx (mpsc) ──► Thread Creation Requests      │    │
│  │    └── open_callback_rx (mpsc) ──► Thread Open Requests     │    │
│  └─────────┬──────────────────────────────┬────────────────────┘    │
│            │                              │                         │
│    ┌───────▼────────┐            ┌────────▼───────┐                 │
│    │ Creation Path  │            │  Open Path     │                 │
│    │ (chat_message) │            │ (open_thread)  │                 │
│    └───────┬────────┘            └────────┬───────┘                 │
│            │                              │                         │
│            ▼                              ▼                         │
│    ┌──────────────┐              ┌──────────────────┐               │
│    │ Has existing │──no──►       │ In registry?     │               │
│    │ thread_id?   │       │      └────────┬─────────┘               │
│    └──────┬───────┘       │          yes/ \no                       │
│           │yes            │          │     │                        │
│           ▼               │          │     ▼                        │
│    ┌──────────────┐       │     return   ┌──────────────────┐       │
│    │ In registry? │       │     early    │ open_existing_   │       │
│    └──────┬───────┘       │     (NO      │ thread_sync()    │       │
│      yes/ \no             │     SUB!)    │                  │       │
│      │     │              │      ▲       │ • loads thread   │       │
│      │     ▼              │      │       │ • registers it   │       │
│      │  ┌──────────────┐  │   BUG!──────►│ • NO subscription│       │
│      │  │ load_thread_ │  │              └──────────────────┘       │
│      │  │ from_agent() │  │                                         │
│      │  │              │  │                                         │
│      │  │ • loads from │  │                                         │
│      │  │   agent DB   │  │                                         │
│      │  │ • registers  │  │                                         │
│      │  │ • subscribes │  │                                         │
│      │  └──────┬───────┘  │                                         │
│      │         │          │                                         │
│      │    ┌────▼────┐     │                                         │
│      └───►│ handle_ │◄────┘                                         │
│           │ follow_ │                                               │
│           │ up_msg()│                                               │
│           └────┬────┘                                               │
│                │                                                    │
│                ▼                                                    │
│    ┌──────────────────────────┐     ┌───────────────────────┐       │
│    │ has_persistent_sub?      │─yes─│ Skip (already have    │       │
│    │                          │     │ subscription)         │       │
│    └──────────┬───────────────┘     └───────────────────────┘       │
│               │no                                                   │
│               ▼                                                     │
│    ┌──────────────────────────┐                                     │
│    │ Create fallback          │                                     │
│    │ subscription             │                                     │
│    └──────────────────────────┘                                     │
│                                                                     │
│    4 functions × up to 3 events = event subscription matrix below   │
└─────────────────────────────────────────────────────────────────────┘
```

## Event Subscription Matrix (before fix)

Three `AcpThreadEvent` variants matter for Helix sync. Each subscription
site either handles (✅), silently discards (❌), or partially handles (⚠️) them.

| Event | `create_new_thread_sync` | `handle_follow_up_message` | `load_thread_from_agent` | `open_existing_thread_sync` |
|---|---|---|---|---|
| `NewEntry` | ✅ user + assistant | ✅ user + assistant | ⚠️ **user only** | ❌ **no subscription** |
| `EntryUpdated` | ✅ assistant + tool_call | ✅ assistant + tool_call | ✅ assistant + tool_call | ❌ **no subscription** |
| `Stopped` | ✅ flush + message_completed | ❌ **`_ => {}`** | ✅ flush + message_completed | ❌ **no subscription** |

## What Each Event Does

### `NewEntry`
A new entry was added to the thread (user message, assistant message, or tool call).
Handler sends `message_added` with the full content of the new entry.
- Checks `is_external_originated_entry()` to avoid echoing back messages we sent.
- `create_new_thread_sync` and `handle_follow_up_message` handle both user + assistant.
- `load_thread_from_agent` only handles user messages (minor inconsistency — assistant
  NewEntry has empty content anyway, so EntryUpdated carries the real data).

### `EntryUpdated(entry_idx)`
An existing entry's content changed (LLM streaming tokens, tool call status update).
Handler calls `throttled_send_message_added()` which rate-limits to 100ms per entry,
storing pending content that gets flushed on `Stopped`.

### `Stopped`
The agentic turn completed (LLM returned `EndTurn`). Handler MUST:
1. Call `flush_streaming_throttle()` — sends any pending throttled content (last ~100ms of tokens)
2. Send `SyncEvent::MessageCompleted` — tells Go API the turn is done

**Without this handler, the last throttled tokens are lost and the frontend never
knows the response finished.**

## Bug 1: Missing `Stopped` Handler (FIXED)

After a sandbox restart:
1. Zed process restarts → `PERSISTENT_SUBSCRIPTIONS` static resets to empty
2. Thread entity may survive in `THREAD_REGISTRY` (or get re-loaded via `open_existing_thread_sync`)
3. `open_existing_thread_sync` finds thread in registry → returns early, **no subscription created**
4. User sends follow-up message from Helix chat sidebar
5. `handle_follow_up_message` checks `has_persistent_subscription()` → false
6. Creates fallback subscription — but it had `_ => {}` catching `Stopped`
7. LLM streams response, `EntryUpdated` events flow correctly (truncated at 100ms throttle)
8. LLM finishes, `Stopped` fires → **silently discarded**
9. Result: last ~100ms of tokens never flushed, `message_completed` never sent
10. Frontend shows truncated response, stuck in "streaming" state forever

### Fix (applied)

1. Extracted all subscription logic into a single `ensure_thread_subscription()` function
2. All 4 call sites now use it — consistent event handling everywhere
3. `open_existing_thread_sync` now creates a subscription even when thread is already in registry

## Bug 2: MessageAccumulator Drops Out-of-Order Flush Updates (CURRENT BUG)

### Root Cause

The `MessageAccumulator` in `api/pkg/server/wsprotocol/accumulator.go` only tracks
a single `LastMessageID` + `Offset`. It can overwrite the **last** message_id's content,
or append a **new** message_id. But it cannot go back and fix an earlier message_id.

This matters because of how Zed structures thread entries:

```
entry_idx 0: UserMessage      → message_id "0" (user prompt)
entry_idx 1: UserMessage      → message_id "1" (echoed user msg)
entry_idx 2: AssistantMessage  → message_id "2" (text: "I'll start by exploring...")
entry_idx 3: ToolCall          → message_id "3" (List directory)
entry_idx 4: ToolCall          → message_id "4" (List directory)
entry_idx 5: ToolCall          → message_id "5" (Read file)
entry_idx 6: AssistantMessage  → message_id "6" (text: "The repo is very...")
...
entry_idx 18: AssistantMessage → message_id "18" (final summary)
```

Each entry has its own `entry_idx` which becomes the `message_id` in `message_added` events.
The 100ms throttle captures a snapshot of each entry's content at the moment `EntryUpdated` fires.

**The problem:** When a tool call arrives, Zed creates a new entry (new entry_idx). The
previous assistant message entry may have been mid-word when the throttle last captured it.
For example:

```
08:22:58  message_added(id="2", content="I'll start...understand the c")  ← TRUNCATED
08:22:59  message_added(id="3", content="**Tool Call: List the `clea`...")  ← TRUNCATED
08:22:59  message_added(id="4", content="**Tool Call: List the `helix-specs/d`...") ← TRUNCATED
...many more entries...
08:23:59  message_added(id="18", content="The design docs have been pushed...") ← streaming
```

Then `Stopped` fires and `flush_streaming_throttle()` sends corrected content for ALL entries:

```
08:23:59  message_added(id="2", content="I'll start...understand the codebase structure...")  ← FULL
08:23:59  message_added(id="3", content="**Tool Call: List the `clean-truncation-test`...")    ← FULL
08:23:59  message_added(id="6", content="The repo is very minimal...")                         ← FULL
```

But the accumulator processes these flush messages AFTER id="18" was already the LastMessageID.
When id="2" arrives again:

```go
// a.LastMessageID = "18", messageID = "2"
// "18" != "2", so this is treated as a NEW message → APPEND
a.Content = a.Content + "\n\n" + content  // WRONG! Should replace id="2"'s content
```

Result: the truncated "understand the c" stays in position, and the corrected
"understand the codebase structure..." is appended at the end as a duplicate.
The final DB content has BOTH the truncated AND the full versions.

### Evidence from Zed WebSocket Logs

```
# During streaming (throttled, content truncated mid-word):
08:22:58  id="2"  "I'll start by exploring the project repository to understand the c"
08:22:59  id="3"  "**Tool Call: List the `clea` directory's contents**"

# During Stopped flush (complete content):
08:23:59  id="2"  "I'll start by exploring the project repository to understand the codebase structure, tech stack, and patterns before writing the spec documents."
08:23:59  id="3"  "**Tool Call: List the `clean-truncation-test` directory's contents**"
```

The complete content IS sent. The accumulator just can't apply it correctly.

### Why Zed's UI Doesn't Have This Problem

Zed renders directly from live `Entity<Markdown>` buffers via GPUI subscriptions.
When the buffer content changes, GPUI re-renders automatically — it always shows
the latest state. There is no snapshot/accumulate/patch pipeline.

Our WebSocket sync path captures snapshots of `content_only()` / `to_markdown()`
at the moment `EntryUpdated` fires, throttled to 100ms. The snapshots can be
mid-word because the Markdown buffer is still being populated by streaming tokens.

### Fix Required

The `MessageAccumulator` needs to support updating ANY message_id, not just the last one.
It must track the byte offset range for each message_id so that flush updates can
replace content at the correct position. Alternatively, the accumulator could use a
map of `message_id → content` and reconstruct the full string on each update.

## Throttle Chain (and where content gets mangled)

```
Zed AcpThread                    Zed thread_service              Go API Server
─────────────                    ──────────────────              ─────────────
EntryUpdated ──100ms throttle──► message_added ──WebSocket──►   handleMessageAdded
  (every token)   (pending if     (full content                   ├─ MessageAccumulator
                   <100ms since    for entry,                     │   ├─ same id → overwrite ✅
                   last send)      may be mid-word                │   └─ diff id → append ✅
                                   if entry is still              │       (but can't go BACK ❌)
                                   streaming when                 ├─ 200ms DB throttle
                                   throttle fires)                └─ 50ms frontend publish
                                                                     (interaction_patch delta)

Stopped ──► flush_streaming_throttle()
              ├─ message_added(id=2, FULL)  ──► accumulator sees id≠last → APPENDS (BUG!)
              ├─ message_added(id=3, FULL)  ──► accumulator sees id≠last → APPENDS (BUG!)
              ├─ message_added(id=6, FULL)  ──► accumulator sees id≠last → APPENDS (BUG!)
              └─ ...
            send MessageCompleted ──► message_completed
                                        ├─ flushAndClearStreamingContext (DB write if dirty)
                                        ├─ reload interaction from DB (has mangled content!)
                                        ├─ mark complete
                                        └─ publish interaction_update to frontend
```

## Call Sites for `setup_thread_handler`

```
zed.rs (workspace creation)
  └── setup_thread_handler(project, acp_history_store, fs, cx)
        │
        ├── mpsc channel 1: ThreadCreationRequest
        │     Producers: WebSocket handler receives `chat_message` from Go API
        │     Consumer: loops in setup_thread_handler, dispatches to:
        │       ├── create_new_thread_sync()     — new thread, no existing ID
        │       ├── handle_follow_up_message()   — existing thread in registry
        │       └── load_thread_from_agent()     — not in registry, load + follow-up
        │
        └── mpsc channel 2: ThreadOpenRequest
              Producers: WebSocket handler receives `open_thread` from Go API
              Consumer: loops in setup_thread_handler, dispatches to:
                └── open_existing_thread_sync()  — display existing thread in UI
```
