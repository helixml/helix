# Thread Subscription Architecture

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

## The Bug

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

## The Fix

1. Add `Stopped` handler to `handle_follow_up_message` fallback subscription (immediate fix)
2. Extract all subscription logic into a single `subscribe_to_thread_events()` function
3. Ensure `open_existing_thread_sync` also creates a subscription (or at minimum ensures one exists)
4. Fix `load_thread_from_agent` `NewEntry` to handle assistant messages too (consistency)

## Throttle Chain

```
Zed AcpThread                    Zed thread_service              Go API Server
─────────────                    ──────────────────              ─────────────
EntryUpdated ──100ms throttle──► message_added ──WebSocket──►   handleMessageAdded
  (every token)   (pending if     (full content                   ├─ in-memory update
                   <100ms since    for entry)                     ├─ 200ms DB throttle
                   last send)                                     └─ 50ms frontend publish
                                                                     (interaction_patch delta)

Stopped ──► flush_streaming_throttle() ──► message_added (final content)
            send MessageCompleted      ──► message_completed
                                              ├─ flushAndClearStreamingContext (DB write if dirty)
                                              ├─ reload interaction from DB
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
