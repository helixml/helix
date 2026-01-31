# Design Review Comment Queue Architecture

**Date**: 2025-12-18
**Status**: Fixed
**Issue**: Multi-minute delays between processing queued comments
**Root Cause**: Session ID mismatch - `handleMessageCompleted` used wrong session ID for comment lookup
**Fix**: Extract `request_id` from message data instead of session-based lookup

## Overview

The design review comment queue allows users to submit comments on spec task design documents. These comments are queued and sent to the AI agent one at a time (serialized) for processing.

## Architecture

### Database Schema

Comments have the following queue-related fields (in `types.SpecTaskDesignReviewComment`):
- `QueuedAt *time.Time` - When the comment was queued for processing (null = not queued)
- `RequestID string` - The request ID for the current agent call (empty = not being processed)
- `AgentResponse string` - The agent's response
- `AgentResponseAt *time.Time` - When the response was received

### Queue States

A comment can be in one of these states:
1. **Not queued**: `QueuedAt = nil, RequestID = ""`
2. **Queued, waiting**: `QueuedAt != nil, RequestID = ""`
3. **Being processed**: `QueuedAt != nil, RequestID = "req_xxx"`
4. **Completed**: `QueuedAt = nil, RequestID = "", AgentResponse != ""`

### Key Functions

#### `queueCommentForAgent` (spec_task_design_review_handlers.go:620)
- Sets `QueuedAt` on the comment
- Updates comment in database
- Calls `go processNextCommentInQueue(sessionID)`

#### `processNextCommentInQueue` (spec_task_design_review_handlers.go:673)
- Checks `IsCommentBeingProcessedForSession` - if already processing, returns
- Gets next queued comment via `GetNextQueuedCommentForSession`
- Calls `sendCommentToAgentNow`
- Sets a 2-minute timeout for agent response

#### `sendCommentToAgentNow` (spec_task_design_review_handlers.go:837)
- Builds prompt from comment
- Sends via WebSocket to agent
- Sets `RequestID` on the comment (marks it as "being processed")

#### `handleMessageCompleted` (websocket_external_agent_sync.go:1412)
- Called when agent sends `message_completed` event
- Looks up `helixSessionID` from `acp_thread_id` via contextMappings
- Queries `GetPendingCommentByPlanningSessionID(helixSessionID)`
- If found, calls `finalizeCommentResponse(requestID)`

#### `finalizeCommentResponse` (spec_task_design_review_handlers.go:990)
- Clears `RequestID` and `QueuedAt`
- Cancels the 2-minute timeout
- Calls `processNextCommentInQueue` for the next comment

## Data Flow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         COMMENT QUEUE FLOW                                    │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User submits comment                                                        │
│         │                                                                    │
│         ▼                                                                    │
│  createDesignReviewComment()                                                 │
│         │                                                                    │
│         ▼                                                                    │
│  queueCommentForAgent()                                                      │
│     - Sets QueuedAt                                                          │
│     - Triggers processNextCommentInQueue                                     │
│         │                                                                    │
│         ▼                                                                    │
│  processNextCommentInQueue()                                                 │
│     - Checks IsCommentBeingProcessedForSession                               │
│     - If already processing → return (wait)                                  │
│     - Gets next queued comment                                               │
│     - Calls sendCommentToAgentNow                                            │
│     - Starts 2-minute timeout                                                │
│         │                                                                    │
│         ▼                                                                    │
│  sendCommentToAgentNow()                                                     │
│     - Sets RequestID on comment                                              │
│     - Sends to agent via WebSocket                                           │
│         │                                                                    │
│    ═════╧═══════════════════════════════════════════════════                 │
│    ║   Agent processes message, sends message_completed    ║                 │
│    ═════╤═══════════════════════════════════════════════════                 │
│         │                                                                    │
│         ▼                                                                    │
│  handleMessageCompleted()                                                    │
│     - Derives helixSessionID from acp_thread_id                              │
│     - Calls GetPendingCommentByPlanningSessionID(helixSessionID) ◄─── BUG?   │
│     - If found → finalizeCommentResponse                                     │
│         │                                                                    │
│         ▼                                                                    │
│  finalizeCommentResponse()                                                   │
│     - Clears RequestID and QueuedAt                                          │
│     - Cancels timeout                                                        │
│     - Calls processNextCommentInQueue → PROCESS NEXT                         │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Identified Issue: Session ID Mismatch

### The Bug

In `handleMessageCompleted`:
```go
// helixSessionID is derived from acp_thread_id → contextMappings
helixSessionID := apiServer.contextMappings[acpThreadID]

// This queries: spec_tasks.planning_session_id = helixSessionID
pendingComment, err := apiServer.Store.GetPendingCommentByPlanningSessionID(context.Background(), helixSessionID)
```

**Problem**: `helixSessionID` is the Helix session associated with the Zed thread. But `GetPendingCommentByPlanningSessionID` looks for comments where `spec_tasks.planning_session_id` matches.

**These may be DIFFERENT IDs!**

- The `planning_session_id` on a spec_task is set when the spec task is created
- The `helixSessionID` from `message_completed` is the session associated with the Zed thread

If the Zed agent reconnects or a new thread is created, the `helixSessionID` would be different from the original `planning_session_id`.

### Evidence

The 2-minute timeout IS what eventually triggers the next comment. This is because:
1. `handleMessageCompleted` calls `GetPendingCommentByPlanningSessionID(helixSessionID)`
2. Query returns nothing (session ID mismatch)
3. `finalizeCommentResponse` is NOT called
4. Queue sits idle until 2-minute timeout fires
5. Timeout clears the comment and calls `processNextCommentInQueue`

### Root Cause

`GetPendingCommentByPlanningSessionID` is the wrong approach. We should use the `RequestID` that's already on the comment to finalize it, not try to look it up by session ID.

The current flow:
1. Comment has `RequestID = "req_xxx"` set by `sendCommentToAgentNow`
2. Agent response arrives with same `RequestID` (or we can derive it)
3. We look up comment by planning_session_id (WRONG - may not match)

The correct flow:
1. Comment has `RequestID = "req_xxx"` set by `sendCommentToAgentNow`
2. Agent sends `message_completed` with `request_id` in the data
3. We call `finalizeCommentResponse(request_id)` directly

### Actually Wait...

Looking at the `sendCommentToAgentNow` function, the `requestID` IS set on the comment. And `message_completed` from the agent DOES include the `request_id` in the message data... but we're not extracting it!

Let me check the message_completed handler more carefully.

## Fix Implemented

The issue was in `handleMessageCompleted`: it was using `GetPendingCommentByPlanningSessionID(helixSessionID)` where `helixSessionID` came from `acp_thread_id` via `contextMappings`, but this didn't match `spec_tasks.planning_session_id`.

**Fix (websocket_external_agent_sync.go:1539-1615):**

1. **Primary approach**: Extract `request_id` directly from `syncMsg.Data["request_id"]`
   - This is the `request_id` we sent with the comment, echoed back by the agent
   - Use it directly to call `finalizeCommentResponse(requestID)`
   - This is the definitive link and bypasses the session ID mismatch issue

2. **Fallback**: Session-based lookup for backwards compatibility
   - Only used if `request_id` is not in the message data
   - May still fail due to session ID mismatch (2-minute timeout will handle it)

**Result**: When the agent echoes back the `request_id`, comments are finalized immediately instead of waiting for the 2-minute timeout.

## Timeout Mechanism

There IS a 2-minute timeout (line 750) that:
- Fires if agent doesn't respond
- Sets `AgentResponse = "[Agent did not respond - try sending your comment again]"`
- Clears `RequestID` and `QueuedAt`
- Calls `processNextCommentInQueue`

This is the fallback that eventually processes the next comment after 2 minutes.
