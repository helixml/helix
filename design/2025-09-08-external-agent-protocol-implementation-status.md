# ⚠️ OUT OF DATE - DO NOT USE ⚠️

**THIS DOCUMENT IS OUTDATED AND KEPT FOR HISTORICAL REFERENCE ONLY**

**See the authoritative spec at:** `/home/luke/pm/zed/WEBSOCKET_PROTOCOL_SPEC.md`

---

# External Agent WebSocket Protocol - Implementation Status

## Completed Changes

### Helix Side

1. ✅ **Updated chat_message payload** (`session_handlers.go:1217`)
   - Now sends `helix_session_id` instead of `session_id`
   - Includes `zed_context_id` from `session.Metadata.ZedThreadID` (null on first message)

2. ✅ **Updated handleContextCreated** (`websocket_external_agent_sync.go:441`)
   - Stores `zed_context_id` on session metadata when received
   - Updates session in database with the mapping

3. ✅ **Added handleMessageCompleted** (`websocket_external_agent_sync.go:997`)
   - New handler for `message_completed` event
   - Finds most recent Waiting interaction
   - Marks it as Complete with timestamp

4. ✅ **Updated handleMessageAdded** (`websocket_external_agent_sync.go:680`)
   - Updates interaction response content
   - **Keeps state as Waiting** (doesn't mark complete)
   - Only `message_completed` marks interaction complete

5. ✅ **Session model already has ZedThreadID field** (`types.go:340`)
   - Field: `Metadata.ZedThreadID`
   - JSON: `zed_thread_id`

### Zed Side

1. ✅ **Updated chat_message handler** (`websocket_sync.rs:497`)
   - Extracts both `helix_session_id` and `zed_context_id`
   - Passes them to `handle_chat_message_with_response`

2. ✅ **Updated handle_chat_message_with_response** (`websocket_sync.rs:826`)
   - Accepts `zed_context_id: Option<String>` parameter
   - Creates `CreateThreadFromExternalSession` event with zed_context_id

3. ✅ **Added zed_context_id to SyncEvent** (`types.rs:156`)
   - `CreateThreadFromExternalSession` now includes `zed_context_id: Option<String>`

4. ✅ **Added zed_context_id to CreateThreadRequest** (`external_websocket_sync.rs:48`)
   - Struct now includes `zed_context_id: Option<String>` field

5. ✅ **Updated WebSocket event processor** (`websocket_sync.rs:210`)
   - Passes `zed_context_id` when creating `CreateThreadRequest`

6. ✅ **Updated agent_panel** (`agent_panel.rs:460`)
   - Checks if `zed_context_id` is provided in request
   - If provided, uses it directly (stateless!)
   - If not, falls back to checking ExternalSessionMapping

7. ✅ **Removed synthetic context_created** (`websocket_sync.rs:234`)
   - No longer generates fake context IDs
   - Only real context_created from agent_panel is sent

## Still TODO

### Critical for Basic Flow

1. ⚠️ **message_completed emission in Zed**
   - Need to send `message_completed` event when AI finishes responding
   - Should include `helix_session_id`, `zed_context_id`, `message_id`, `request_id`
   - Currently not implemented - messages will stay in Waiting state

2. ⚠️ **Add message to existing context in agent_panel**
   - When `zed_context_id` is provided (follow-up message)
   - Currently just logs "TODO: Add message to existing context"
   - Need to actually add the message to the context

3. ⚠️ **Remove unnecessary acknowledgment**
   - Still sending placeholder acknowledgment in chat_message handler
   - Should be removed as part of protocol cleanup

### Non-Critical / Nice to Have

4. **Clean up old context mapping code**
   - `ExternalSessionMapping` is still being maintained
   - Could be removed since Helix now provides zed_context_id

5. **Remove context_to_helix_session mapping**
   - No longer needed with new protocol
   - Zed receives helix_session_id in every message

6. **Clean up sessionToWaitingInteraction**
   - Added earlier as attempted fix
   - May not be needed with message_completed

## Testing Plan

1. **Test Flow A: New Session**
   - Create session in Helix
   - Send first message
   - Verify context_created is received
   - Verify message_added events arrive
   - Verify interaction marked complete when done

2. **Test Flow B: Follow-up Message**
   - Send second message in same session
   - Verify zed_context_id is included
   - Verify message goes to same Zed thread
   - Verify response appears correctly

3. **Test Flow C: Streaming**
   - Verify multiple message_added with same message_id
   - Verify content grows progressively
   - Verify interaction stays Waiting until completed

## Build Commands

```bash
# Helix API (hot reload enabled)
cd /home/luke/pm/helix
docker compose -f docker-compose.dev.yaml restart api

# Zed
cd /home/luke/pm/helix
./stack build-zed
# Then close and reopen Zed window in VNC (auto-restart picks up new binary)
```
