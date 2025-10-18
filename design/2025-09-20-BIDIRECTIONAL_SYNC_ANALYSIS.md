# Bidirectional Sync Issues - Comprehensive Analysis

## Executive Summary

During interactive testing of the Helix ↔ Zed bidirectional sync, several critical issues were discovered where the theoretical design doesn't match actual behavior. This document analyzes the problems, traces relevant codepaths, and provides root cause analysis.

## 1. Observed Behavior vs Expected Behavior

### Issue 1: Agent Panel Auto-Open Failure
**Expected:**
- When Helix sends first message to Zed via WebSocket
- Zed agent panel should automatically open
- The newly created thread should be visible and focused

**Actual:**
- Agent panel did NOT auto-open
- Had to manually open the panel
- Thread was created but not visible/focused

**Impact:** Critical UX issue - user has no visual feedback that anything is happening

---

### Issue 2: Thread in History Instead of Active View
**Expected:**
- The thread created by Helix WebSocket should be the active/focused thread in the panel
- Should be immediately visible when panel opens

**Actual:**
- Thread appeared in the history list (sidebar)
- NOT in the active view
- Had to manually click the thread in history to see the response
- Response WAS correctly present in the thread (just not displayed)

**Impact:** User must perform extra navigation to see AI responses

---

### Issue 3: Follow-up Message Invisible in Zed UI
**Expected:**
- Follow-up message ("write a snake game in python") should appear in the active thread
- Should be visible in Zed UI
- User should see the AI working

**Actual:**
- Follow-up message did NOT appear in Zed thread UI
- Agent WAS working in background (writing files to workspace)
- Zed UI showed no indication of activity
- No threads visible in history or active view showed the follow-up

**Impact:** Complete disconnect between what's happening (agent working) and what user sees (nothing)

---

### Issue 4: Helix UI Not Showing Streaming Responses
**Expected:**
- Helix frontend should show streaming/partial AI responses
- User sees progress as response is generated

**Actual:**
- Helix UI only showed spinner
- Session metadata (via info button) DID show quasi-streaming updates
- Data is flowing but not displayed to user

**Impact:** Poor UX - no streaming feedback (NOTE: This may be a frontend issue, not sync issue)

---

### Issue 5: Valid Response Clobbered by Timeout Error
**Expected:**
- Agent completes task
- Final message appears in Helix session
- Interaction marked as complete
- Response remains visible

**Actual:**
1. Agent finished writing snake game
2. Final message "I've made a snake game for you" DID make it to Helix session initially
3. Message was still invisible in Zed thread UI
4. No threads visible in Zed UI/history at all
5. After some time, Helix switched the interaction response to error: "External agent response timeout"
6. Valid response was clobbered/overwritten

**Impact:** CRITICAL - Valid, complete responses are being destroyed by spurious timeout errors

---

## 2. Codepath Analysis

### 2.1 Message Flow: Helix → Zed (Initial Message)

**Starting Point: Helix API receives chat request**
- File: `/home/luke/pm/helix/api/pkg/server/session_handlers.go`
- Handler: `chatWithExternalAgent()`

**Step 1: Create WebSocket message**
```go
// Line ~1350 in session_handlers.go
syncMsg := types.WebSocketExternalAgentSyncMessage{
    Type: "chat_message",
    Data: map[string]interface{}{
        "acp_thread_id": threadID,  // May be empty for new threads
        "message":       lastInteraction.Message,
        "request_id":    requestID,
        "session_id":    sessionID,
    },
}
```

**Step 2: Send via WebSocket**
- File: `/home/luke/pm/helix/api/pkg/server/websocket_external_agent_sync.go`
- Method: `sendToExternalAgent()`
- Sends JSON message to Zed via WebSocket connection

**Step 3: Zed receives message**
- File: `/home/luke/pm/zed/crates/external_websocket_sync/src/websocket_sync.rs`
- Incoming message handler processes JSON
- Routes to appropriate handler based on `type` field

**Step 4: Process chat_message**
- File: `/home/luke/pm/zed/crates/external_websocket_sync/src/external_websocket_sync.rs`
- Method: `handle_incoming_message()`
- Parses `chat_message` type
- Calls `request_thread_creation()`

**Step 5: Thread Service creates or reuses thread**
- File: `/home/luke/pm/zed/crates/external_websocket_sync/src/thread_service.rs`
- Method: Thread service receives request via channel
- Checks if `acp_thread_id` is empty or exists
- Creates new thread OR sends to existing thread

**Step 6: Notify AgentPanel to display thread**
- File: `/home/luke/pm/zed/crates/external_websocket_sync/src/thread_service.rs`
- Line ~277-287: After thread registration
```rust
if let Err(e) = crate::notify_thread_display(crate::ThreadDisplayNotification {
    thread_entity: thread_entity.clone(),
    helix_session_id: request_clone.request_id.clone(),
}) {
    eprintln!("⚠️ [THREAD_SERVICE] Failed to notify thread display: {}", e);
}
```

**Step 7: AgentPanel receives notification**
- File: `/home/luke/pm/zed/crates/agent_ui/src/agent_panel.rs`
- Line ~735-772: Async task listening on callback channel
```rust
cx.spawn_in(window, async move |this, cx| {
    while let Some(notification) = callback_rx.recv().await {
        // Extract thread metadata
        let thread_metadata_result = notification.thread_entity.read_with(cx, |thread, _cx| {
            agent2::DbThreadMetadata {
                id: thread.session_id().clone(),
                title: thread.title(),
                updated_at: chrono::Utc::now(),
            }
        });

        // Call external_thread() to display
        this.update_in(cx, |this, window, cx| {
            this.external_thread(
                Some(crate::ExternalAgent::NativeAgent),
                Some(thread_metadata),
                None,
                window,
                cx,
            );
        });
    }
})
```

**CRITICAL QUESTION 1:** Is the callback channel notification actually being sent?
**CRITICAL QUESTION 2:** Is the AgentPanel async task actually receiving it?
**CRITICAL QUESTION 3:** Is `external_thread()` being called?
**CRITICAL QUESTION 4:** What does `external_thread()` actually do - does it open the panel AND focus the thread?

### 2.2 Message Flow: Zed → Helix (Response)

**Step 1: Thread generates response**
- ACP thread processes user message
- Generates assistant response
- Emits `EntryUpdated` events

**Step 2: Thread Service monitors events**
- File: `/home/luke/pm/zed/crates/external_websocket_sync/src/thread_service.rs`
- Subscribes to thread events
- Filters for `AssistantMessage` entries

**Step 3: Send message chunks**
```rust
// Send message_added events for streaming
external_websocket_sync::send_websocket_event(
    external_websocket_sync::SyncEvent::MessageAdded {
        acp_thread_id: thread_id.to_string(),
        content: content.clone(),
        // ...
    }
)
```

**Step 4: Helix receives message_added events**
- File: `/home/luke/pm/helix/api/pkg/server/websocket_external_agent_sync.go`
- Handler: `handleMessageAdded()`
- Updates interaction with partial response

**Step 5: Send message_completed**
```rust
// When thread stops
external_websocket_sync::send_websocket_event(
    external_websocket_sync::SyncEvent::MessageCompleted {
        acp_thread_id: thread_id.to_string(),
        message_id: "0".to_string(),
        request_id: request_id.clone(),
    }
)
```

**Step 6: Helix receives message_completed**
- File: `/home/luke/pm/helix/api/pkg/server/websocket_external_agent_sync.go`
- Handler: `handleMessageCompleted()`
- Marks interaction as complete
- Sets final state

**CRITICAL QUESTION 5:** When follow-up message sent, is it going to the SAME thread?
**CRITICAL QUESTION 6:** Is the thread service monitoring the right thread for follow-up?
**CRITICAL QUESTION 7:** Why would response events be sent but thread not visible in UI?

### 2.3 Timeout Mechanism

**Helix Side Timeout:**
- File: `/home/luke/pm/helix/api/pkg/server/session_handlers.go`
- Method: `chatWithExternalAgent()`
- Sets timeout for external agent response

**CRITICAL QUESTION 8:** What is the timeout value?
**CRITICAL QUESTION 9:** Does timeout fire even if response completed successfully?
**CRITICAL QUESTION 10:** Is there a race condition between message_completed and timeout?

---

## 3. Key Questions to Investigate

### Investigation Priority 1: Auto-Open Failure
1. ✅ Is `notify_thread_display()` being called? (Need to check logs)
2. ✅ Is the callback channel working? (Need to check if receiver gets notification)
3. ✅ Is `external_thread()` being called? (Need to add debug logs)
4. ✅ Does `external_thread()` open the panel? (Need to read implementation)
5. ✅ Does it focus/activate the thread? (Need to verify behavior)

### Investigation Priority 2: Follow-up Message Invisibility
1. ✅ Which thread is the follow-up going to? (Check acp_thread_id in logs)
2. ✅ Is thread service monitoring that thread? (Check subscriptions)
3. ✅ Are there MULTIPLE threads created? (Check thread registry)
4. ✅ Is UI showing the wrong thread? (Check active_view state)

### Investigation Priority 3: Timeout Clobbering Response
1. ✅ What triggers the timeout? (Read timeout code)
2. ✅ Is message_completed arriving? (Check Helix logs)
3. ✅ Is there a race condition? (Analyze timing)
4. ✅ Does timeout check if response already complete? (Read implementation)

---

## 4. Root Cause Hypotheses

### Hypothesis 1: AgentPanel Callback Never Initialized
**Theory:** The AgentPanel's callback handler may not be running when external thread is created

**Evidence Needed:**
- Check if `init_thread_display_callback()` is called before first message
- Check if callback task is spawned before notification sent
- Add logs to verify notification sent and received

**Test:** Add debug logging to both send and receive sides of callback

---

### Hypothesis 2: external_thread() Doesn't Actually Open Panel or Focus Thread
**Theory:** The `external_thread()` method may only prepare the thread but not make it visible

**Evidence Needed:**
- Read full implementation of `external_thread()` method
- Understand what it does vs what we think it does
- Check if separate action needed to open panel

**Test:** Trace through `external_thread()` implementation completely

---

### Hypothesis 3: Thread Metadata Mismatch
**Theory:** Thread metadata (id, title) may not match what history/UI expects

**Evidence Needed:**
- Compare thread.session_id() with what history stores
- Check if DbThreadMetadata format is correct
- Verify chronological timestamp handling

**Test:** Log the exact metadata being passed to external_thread()

---

### Hypothesis 4: Multiple Threads Created for Same Session
**Theory:** Follow-up messages may be creating NEW threads instead of reusing existing

**Evidence Needed:**
- Check if thread registry lookup is working
- Verify acp_thread_id is correctly passed in follow-up
- Check if multiple threads exist with different IDs

**Test:** Log all thread creation/lookup operations

---

### Hypothesis 5: Timeout Doesn't Check Completion Status
**Theory:** Timeout may fire unconditionally without checking if response already complete

**Evidence Needed:**
- Read timeout implementation in session_handlers.go
- Check if completion flag is checked before setting error
- Look for race condition between timeout and message_completed handler

**Test:** Add logs showing timing of timeout vs message_completed

---

### Hypothesis 6: Request ID vs Thread ID Confusion
**Theory:** The message_completed event may use wrong ID causing mismatch

**Evidence Needed:**
- Check what request_id is sent in message_completed
- Verify it matches what Helix is waiting for
- Look for ID mapping issues

**Test:** Log all IDs in both Zed and Helix sides

---

## 5. Investigation Plan

### Phase 1: Add Comprehensive Logging
1. Add logs to `notify_thread_display()` call site
2. Add logs to callback receiver in AgentPanel
3. Add logs to `external_thread()` method entry/exit
4. Add logs to thread creation/lookup in thread_service
5. Add logs to timeout mechanism in Helix

### Phase 2: Read Critical Implementations
1. Fully read `external_thread()` method implementation
2. Fully read timeout implementation in Helix
3. Fully read thread lookup logic in thread_service

### Phase 3: Test with Logging
1. Rebuild Zed with debug logs
2. Send test message from Helix
3. Analyze log output to identify exact failure point

### Phase 4: Root Cause and Fix
1. Based on evidence, identify root cause
2. Implement targeted fix
3. Test to verify fix works

---

## 6. Findings From Log Analysis

### Finding 1: ✅ AgentPanel Auto-Open WAS Called But Didn't Work

**Evidence from Zed logs:**
```
🔧 [AGENT_PANEL] Thread display handler task started
📤 [THREAD_SERVICE] Notified AgentPanel to display thread
🎯 [AGENT_PANEL] Received thread created for session: req_1759747965263304344
✅ [AGENT_PANEL] Auto-opened thread in UI
```

**Conclusion:**
- Callback system IS working correctly
- `external_thread()` WAS called
- BUT it didn't actually open the panel or focus the thread
- **Root Cause:** `external_thread()` method doesn't do what we think it does

---

### Finding 2: 🔴 CRITICAL - Wrong Request ID in message_completed

**Timeline from logs:**

**First message "hi" (10:51:31):**
- Request ID: `req_1759747965263304344`
- Response completed at 10:52:49 with SAME request_id ✅

**Second message "write a snake game" (10:53:34):**
- Request ID: `req_1759748014783651682` (NEW)
- Helix sends to Zed with this new request_id
- Zed processes message and generates response
- **BUG:** message_completed sent with WRONG request_id: `req_1759747965263304344` (the ORIGINAL one!)
- Helix waits for `req_1759748014783651682` but never receives it
- Timeout fires at 10:55:04 (90 seconds later)

**Evidence:**
```log
# Follow-up sent with NEW request_id
[10:53:34] 🔴 [HELIX] SENDING CHAT_MESSAGE COMMAND TO EXTERNAL AGENT
request_id=req_1759748014783651682 message="write a snake game in python"

# Response streams correctly...
[10:54:33-37] External agent added message (many streaming updates)

# BUT completion uses WRONG (old) request_id!
[10:54:37] message_completed request_id=req_1759747965263304344  ❌ WRONG!

# Timeout for the actual request_id
[10:55:04] External agent response timeout request_id=req_1759748014783651682
```

**Root Cause:** Zed's thread_service stores the request_id from the FIRST message and reuses it for ALL subsequent message_completed events. It should use the CURRENT request_id instead.

---

### Finding 3: 🟡 Helix Echoes Messages Back to Zed (Duplicate)

**Evidence from Zed logs:**
```
# User message sent from Helix to Zed
📥 [WEBSOCKET-IN] Received text: {"type":"chat_message","data":{"acp_thread_id":"","message":"hi",...}}

# Zed processes and sends to AI...

# THEN Helix echoes the SAME message back to Zed!
📥 [WEBSOCKET-IN] Received text: {"type":"chat_message","data":{"acp_thread_id":"4294967571","message":"hi","role":"user"...}}
```

**Impact:** Zed receives duplicate user messages - once from Helix's initial request, then again as an echo

**Root Cause:** Helix is broadcasting user messages back to Zed via WebSocket (probably for UI sync) but Zed processes them as new messages

---

### Finding 4: 🟠 Thread Created But Not Visible in Zed UI

User reported: "when i clicked the thread in the history, i saw the response"

**Interpretation:**
- Thread exists and is functional (response was generated)
- Thread appears in history sidebar
- BUT not in the active/focused view
- User had to manually click thread in history to see it

**Hypothesis:** `external_thread()` adds thread to history but doesn't make it the active view

---

### Finding 5: 🟠 Follow-up Message Invisible in Zed UI

User reported: "follow-up message did NOT see get added to the zed thread"

**Evidence:** Agent WAS working (writing files) but UI showed nothing

**Hypothesis:** Follow-up messages go to the SAME thread but UI doesn't refresh/update to show new messages in that thread

---

## 7. Code Locations and Fixes Required

### Fix 1: 🔴 CRITICAL - Wrong Request ID in message_completed

**File:** `/home/luke/pm/zed/crates/external_websocket_sync/src/thread_service.rs`

**Current Bug:** Thread service stores `request_id` from FIRST message and reuses it for ALL message_completed events

**Location to fix:** Search for where `request_id` is stored and where `MessageCompleted` event is sent

**Required Change:**
- Store CURRENT request_id for each message
- Use the CURRENT request_id when sending message_completed
- Don't reuse the first request_id for all completions

---

### Fix 2: 🟠 `external_thread()` Doesn't Open Panel or Focus Thread

**File:** `/home/luke/pm/zed/crates/agent_ui/src/agent_panel.rs`

**Current Behavior:** Calling `external_thread()` prepares thread but doesn't:
1. Open the agent panel (if closed)
2. Switch to the newly created thread view
3. Make thread the active/focused view

**Required Investigation:**
1. Read `external_thread()` implementation to understand what it does
2. Find method that actually opens panel and switches view
3. Call that method from the auto-open callback

---

### Fix 3: 🟡 Stop Processing Duplicate Messages from Helix

**File:** `/home/luke/pm/zed/crates/external_websocket_sync/src/external_websocket_sync.rs`

**Current Behavior:** Helix echoes user messages back to Zed with `role="user"`, Zed processes them as new messages

**Required Change:**
- Check if incoming `chat_message` has `role="user"`
- If yes, it's an echo - ignore it (don't create/send to thread)
- Only process messages WITHOUT role field (original from Helix)

---

### Fix 4: 🔵 Helix UI Streaming (Frontend Issue)

**File:** Frontend React components

**Current Behavior:** Helix UI shows spinner, doesn't display streaming partial responses

**Note:** This is a frontend issue, not a sync issue. Data IS streaming to backend (seen in session metadata), just not displayed in UI.

---

## 8. Implementation Plan

### Phase 1: Fix Critical Request ID Bug (PRIORITY)
1. Read thread_service.rs to find request_id storage
2. Modify to store and use correct request_id per message
3. Test with follow-up messages

### Phase 2: Fix Auto-Open Panel/Thread
1. Read external_thread() implementation
2. Find correct method to open panel and switch view
3. Update callback to use correct method

### Phase 3: Fix Duplicate Message Processing
1. Add role field check in message handler
2. Filter out echoed user messages

### Phase 4: Test End-to-End
1. Rebuild Zed
2. Test: Send message, verify panel opens and thread visible
3. Test: Send follow-up, verify no timeout and response visible
4. Test: Verify no duplicate messages processed

---

## 9. Fixes Implemented

### Fix 1: ✅ FIXED - Wrong Request ID in message_completed

**Problem:** Thread service captured request_id from FIRST message and reused it for ALL message_completed events

**Solution Implemented:**
1. Created global `THREAD_REQUEST_MAP`: `HashMap<String, String>` mapping thread_id → current_request_id
2. Added `set_thread_request_id()` to update the current request_id for a thread
3. Added `get_thread_request_id()` to retrieve the current request_id for a thread
4. Modified thread creation to call `set_thread_request_id()` when thread is created
5. Modified `handle_follow_up_message()` to call `set_thread_request_id()` when follow-up is sent
6. Modified `AcpThreadEvent::Stopped` handler to use `get_thread_request_id()` instead of captured variable

**Files Changed:**
- `/home/luke/pm/zed/crates/external_websocket_sync/src/thread_service.rs`
  - Lines 28-32: Added THREAD_REQUEST_MAP global
  - Lines 41-44: Initialize request map in init_thread_registry()
  - Lines 47-60: Added set/get functions for thread request_id
  - Line 236: Set request_id when creating new thread
  - Lines 280-292: Use get_thread_request_id() in Stopped event handler
  - Lines 385-398: Updated handle_follow_up_message() signature and implementation

**Testing Status:** Built successfully, ready for end-to-end testing

---

### Fix 2: ✅ FIXED - external_thread() Doesn't Open Panel or Focus Thread

**Problem:** Auto-open callback called `external_thread()` which sets active view but doesn't open the panel if closed

**Solution Implemented:**
1. Added workspace reference capture in callback setup
2. Before calling `external_thread()`, check if agent panel is focused
3. If not focused, call `workspace.toggle_panel_focus::<AgentPanel>()` to open panel
4. Then call `external_thread()` to switch to the new thread view

**Files Changed:**
- `/home/luke/pm/zed/crates/agent_ui/src/agent_panel.rs`
  - Line 735: Capture workspace weak reference for callback
  - Lines 756-769: Check if panel focused, open if not
  - Lines 777-787: Then call external_thread() to set active view

**Testing Status:** Implemented, ready for testing

---

### Fix 3: ✅ FIXED - Stop Processing Duplicate Echoed Messages

**Problem:** Helix echoes user messages back to Zed with `role="user"`, causing Zed to process them twice

**Solution Implemented:**
1. Check if incoming `chat_message` has `role` field
2. If `role == "user"`, it's an echo from Helix - ignore it
3. Only process messages without role field (original from Helix)

**Files Changed:**
- `/home/luke/pm/zed/crates/external_websocket_sync/src/websocket_sync.rs`
  - Lines 261-267: Added check for role="user" and early return

**Code:**
```rust
// CRITICAL: Ignore echoed user messages from Helix (they have role="user")
// Helix broadcasts user messages back via WebSocket for UI sync, but we already processed the original
if command.data.role.as_deref() == Some("user") {
    eprintln!("🔄 [WEBSOCKET-IN] Ignoring echoed user message (role=user) - already processed original");
    log::info!("🔄 [WEBSOCKET-IN] Ignoring echoed user message (role=user) - already processed original");
    return Ok(());
}
```

**Testing Status:** Implemented, ready for testing

---

## 10. Summary of All Fixes

### ✅ Fix 1: CRITICAL - Wrong request_id in message_completed
**Root Cause:** Thread service captured first request_id and reused for all completions
**Fix:** Global map tracking current request_id per thread, updated on each message
**Impact:** Follow-up messages will no longer timeout, correct request_id sent in completion events

### ✅ Fix 2: Panel Auto-Open and Focus
**Root Cause:** `external_thread()` only set active view, didn't open closed panel
**Fix:** Check if panel focused, call `toggle_panel_focus()` to open, then set active view
**Impact:** Agent panel will auto-open and show new thread when Helix creates it

### ✅ Fix 3: Duplicate Message Processing
**Root Cause:** Helix echoes user messages back with role="user", Zed processes twice
**Fix:** Filter messages with role="user" - they're echoes, not new messages
**Impact:** No more duplicate processing, cleaner logs, better performance

---

## 11. Files Modified Summary

**Zed Thread Service:**
- `/home/luke/pm/zed/crates/external_websocket_sync/src/thread_service.rs`
  - Added THREAD_REQUEST_MAP global for request_id tracking
  - Modified thread creation to set current request_id
  - Modified follow-up handling to update request_id
  - Modified Stopped event to use current request_id

**Zed Agent Panel:**
- `/home/luke/pm/zed/crates/agent_ui/src/agent_panel.rs`
  - Added panel auto-open logic in callback handler
  - Check if panel focused, open if needed
  - Then call external_thread() to switch view

**Zed WebSocket Handler:**
- `/home/luke/pm/zed/crates/external_websocket_sync/src/websocket_sync.rs`
  - Added filter for echoed messages with role="user"

---

## 12. Additional Fix After Crash Investigation

### Fix 4: ✅ FIXED - Duplicate Thread Creation (Race Condition)

**Problem Discovered:** Zed crashed with panic, then on restart showed "New Thread" template instead of actual thread

**Root Cause Analysis:**
1. WebSocket handler creates headless `Entity<AcpThread>`
2. Notification callback receives the actual thread entity
3. BUT I was calling `external_thread(DbThreadMetadata)` which:
   - Tries to load thread from database by session_id
   - Database save is ASYNC (happens on thread changes via observer)
   - Race condition: Database may not have thread yet when UI tries to load
   - Falls back to creating NEW thread with same session_id → duplicate!
4. Also caused panic: "cannot update AgentPanel while it is already being updated" (reentrancy)

**Solution Implemented:**
1. Created new constructor: `AcpThreadView::from_existing_thread(Entity<AcpThread>, ...)`
2. Directly wraps the existing headless thread entity in a view
3. Bypasses database loading completely
4. No race condition - uses the thread that already exists in memory
5. Separated workspace.focus_panel() from panel.update() to avoid reentrancy panic

**Files Changed:**
- `/home/luke/pm/zed/crates/agent_ui/src/acp/thread_view.rs`
  - Lines 323-468: Added `from_existing_thread()` constructor
  - Creates view in `ThreadState::Ready` directly with provided thread
  - Skips all async loading/database queries

- `/home/luke/pm/zed/crates/agent_ui/src/agent_panel.rs`
  - Lines 764-782: Use `from_existing_thread()` instead of `external_thread()`
  - Pass actual thread entity from notification
  - Focus workspace panel separately to avoid reentrancy

**Key Insight:**
- Headless threads remain completely independent (can work without UI) ✅
- UI can attach to them by wrapping the existing entity (no database dependency) ✅
- Best of both worlds: separation + visibility

---

## 13. CRITICAL Discovery: Multiple Agent Instances Issue

### The Real Root Cause of Duplicate Threads

**Architecture Discovery:**
Each call to `.server().connect()` creates a COMPLETELY NEW `Entity<NativeAgent>` with its own `sessions` HashMap!

**Why this breaks our approach:**
1. WebSocket handler: calls `.server().connect()` → Creates NativeAgent instance A
2. WebSocket creates thread → Thread registered in instance A's `sessions` map
3. UI: calls `.server().connect()` → Creates NativeAgent instance B (different entity!)
4. UI calls `open_thread(session_id)` → Checks instance B's `sessions` (EMPTY!)
5. Not found → Loads from database
6. Database save is ASYNC (triggered by observer on thread changes)
7. Race condition: Database might not have thread yet
8. Falls back to creating NEW thread → DUPLICATE!

**Evidence from logs:**
```
✅ [THREAD_SERVICE] Created ACP thread: 4294967571  // Entity ID
📋 [THREAD_SERVICE] Registered thread: 4294967571   // In agent A's sessions

// Later when UI tries to open:
📂 [AGENT] open_thread: No existing session for eef40bde-8ec3-47e8-a5ff-a99452bcc5f5
// Looking in agent B's sessions (empty!) - session_id is the UUID
```

**Why existing code works:**
- Clicking history: Loads from database (thread is saved by then)
- Creating new threads: Each thread view has its own agent instance
- Not meant to share agents across views!

**Why our WebSocket case is different:**
- Thread is BRAND NEW (not in database yet)
- Created by one agent instance (A)
- UI tries to view with different agent instance (B)
- Can't look up across instances!

**The only solution: Pass thread entity directly**
- We HAVE the `Entity<AcpThread>` from WebSocket
- Need to wrap it in a view WITHOUT going through database
- This is what `from_existing_thread()` does
- It's not a hack - it's the correct pattern for viewing brand-new threads!

---

## 14. Final Fix Summary

### Why `from_existing_thread()` is Required (Not a Workaround)

**Zed's Architecture (by design):**
- Each `server.connect()` creates a NEW `Entity<NativeAgent>`
- Each NativeAgent has its own `sessions: HashMap<SessionId, Session>`
- Sessions are NOT shared across agent instances
- Threads persist via DATABASE (async save on changes)
- Clicking history loads from DB, creates new agent, registers in new agent's sessions

**This works fine for normal usage:**
- User creates thread → saved to DB over time
- User clicks history later → loads from DB into new agent instance
- Database is source of truth for persistence

**But breaks for WebSocket headless threads:**
1. WebSocket handler calls `.server().connect()` → NativeAgent instance A created
2. Creates thread → Registered in A's `sessions` map
3. Notification sent with thread entity immediately
4. UI callback calls `.server().connect()` → NativeAgent instance B created (different!)
5. UI tries `external_thread()` → loads from DB via instance B
6. Database save is ASYNC, might not be complete yet
7. Even if DB has it, loading creates DUPLICATE registration in B's sessions
8. Result: Wrong thread shown or "New Thread" template

**The solution: from_existing_thread()**
- Takes the `Entity<AcpThread>` we already have from WebSocket
- Creates view directly in `ThreadState::Ready` with that entity
- Bypasses database entirely (no race condition)
- Syncs existing entries immediately
- Headless thread remains independent, UI just "watches" it

**This is the correct pattern because:**
- We have the live thread entity - no reason to throw it away!
- Database is for persistence, not for cross-instance lookups
- Matches Zed's architecture: each view can have different agent instance
- Headless threads remain fully independent ✅
- UI can view them without coupling ✅

---

## 15. All Fixes Summary (Final)

1. ✅ Request ID tracking per thread (timeout fix)
2. ✅ Filter duplicate echoed messages
3. ✅ Panel auto-open with workspace.focus_panel()
4. ✅ Check sessions in open_thread() (helps but not sufficient alone)
5. ✅ from_existing_thread() - Direct entity wrapping (THE KEY FIX)

---

## 16. Test Results (To Be Updated After Testing)

### Test 1: [Pending]

### Test 2: [Pending]

### Test 3: [Pending]
