# Helix ↔ Zed WebSocket Bidirectional Sync - Design Constraints

## Current State (What Works)

### Flow 2: Zed → Helix (User creates thread in Zed)
✅ **Status: WORKING**

1. User inside Zed creates NEW thread (no existing Helix session)
2. Zed sends `thread_created` with `acp_thread_id` and `request_id` (no helix_session_id)
3. API receives `thread_created`, sees NO helix_session_id
4. API creates NEW Helix session for this Zed thread
5. API stores mapping: `acp_thread_id` → `helix_session_id` in `contextMappings`
6. Zed sends `message_added` events → API looks up Helix session via `contextMappings[acp_thread_id]`
7. Frontend receives updates via pubsub

### What's Broken

### Flow 1: Helix → Zed (User creates session in Helix, Zed serves it)
❌ **Status: NOT IMPLEMENTED**

**Problem**: When user creates a Helix session with `agent_type="zed_external"` and sends a message, there's currently no way to:
1. Route that message to the correct Zed instance
2. Have Zed create a thread for that EXISTING Helix session
3. Map the new Zed thread back to the original Helix session

## Design Constraints

### C1: Protocol Constraints
- **Zed must remain stateless regarding Helix**: Zed only knows about `acp_thread_id` (Zed's internal thread IDs)
- **API owns all mappings**: Only the Helix API side stores the relationship between Zed threads and Helix sessions
- **WebSocket protocol uses Zed IDs**: The protocol must only deal in `acp_thread_id`, not `helix_session_id`

### C2: Multi-Session Support
- **One Zed can serve multiple Helix sessions**: A single Zed instance should be reusable across multiple Helix sessions (future requirement)
- **Cannot use env vars for session mapping**: `HELIX_SESSION_ID` env var would lock one Zed to one session

### C3: Current Limitations (Can Be Fixed Later)
- **One Zed instance per Wolf app** (current state): Right now there's 1:1 mapping between Wolf app and Zed container
- **Manual Moonlight connection**: User manually connects via Moonlight client (Wolf auto-start coming later)
- **Session reconciliation**: Wolf apps are "reconciled" from Helix sessions (mechanism TBD)

## Proposed Solution for Flow 1

### Key Insight: Use request_id as the Bridge

When API sends `chat_message` to Zed, it includes a `request_id`. When Zed responds with `thread_created`, it echoes back the same `request_id`. The API can use this to map the new Zed thread to the originating Helix session.

### Data Structures Needed

**On API side (apiServer struct)**:
```go
requestToSessionMapping  map[string]string  // request_id -> helix_session_id
contextMappings          map[string]string  // acp_thread_id -> helix_session_id
agentToSessionMapping    map[string]string  // agent_session_id -> helix_session_id (for routing chat_message)
```

### Complete Flow 1 Implementation

**1. Helix Session Creation**
- User creates session via `/api/v1/sessions` with `Metadata.AgentType = "zed_external"`
- Gets `helix_session_id`
- (Future: Wolf reconciliation creates Wolf app with `agent_session_id`)

**2. User Sends Message**
- User sends message to `helix_session_id`
- API creates `interaction` with `interaction_id` in that session
- API generates `request_id = interaction_id` (or separate UUID)
- API stores: `requestToSessionMapping[request_id] = helix_session_id`

**3. Route Message to Zed**
- API looks up which `agent_session_id` serves this `helix_session_id` via `agentToSessionMapping`
- API sends `chat_message` to that Zed instance WebSocket with:
  ```json
  {
    "type": "chat_message",
    "data": {
      "message": "user's message",
      "request_id": "<request_id>",
      "acp_thread_id": null  // null = create new thread
    }
  }
  ```

**4. Zed Creates Thread**
- Zed receives `chat_message` with `request_id`
- Zed creates new ACP thread → gets `acp_thread_id`
- Zed sends `thread_created`:
  ```json
  {
    "event_type": "thread_created",
    "data": {
      "acp_thread_id": "<zed_thread_id>",
      "request_id": "<request_id>"  // SAME request_id echoed back
    }
  }
  ```

**5. API Maps Thread to Session**
- API receives `thread_created` with `request_id`
- API looks up: `helix_session_id = requestToSessionMapping[request_id]`
- API stores: `contextMappings[acp_thread_id] = helix_session_id`
- API updates Helix session with `Metadata.ZedThreadID = acp_thread_id`
- API marks interaction as "processing"

**6. Zed Sends Response**
- Zed sends `message_added` events with `acp_thread_id`
- API looks up: `helix_session_id = contextMappings[acp_thread_id]`
- API updates interaction `ResponseMessage` in that Helix session
- API publishes to user's pubsub → Frontend receives updates ✅

**7. Zed Completes**
- Zed sends `message_completed` with `acp_thread_id` and `request_id`
- API looks up Helix session, marks interaction complete
- Frontend shows completed state ✅

## Implementation Plan: Simplification for Current Test

**Keep the existing external-agents flow but fix the session reuse:**

1. User calls `/api/v1/external-agents` with `session_id` and user message
   - This creates the Wolf app
   - **NEW**: API creates Helix session with `agent_type="zed_external"` FIRST
   - API stores: `externalAgentSessionMapping[agent_session_id] = helix_session_id`
   - API generates `request_id`, stores: `requestToSessionMapping[request_id] = helix_session_id`

2. Wolf app starts, Zed connects via WebSocket with `?session_id=<agent_session_id>`
   - API tracks WebSocket connection in `externalAgentWSManager`

3. API sends `chat_message` to Zed with the user's initial message
   - Includes `request_id` from step 1
   - Command: `{"type":"chat_message","data":{"message":"...","request_id":"...","acp_thread_id":null}}`

4. Zed responds with `thread_created` including same `request_id`
   - **FIX**: API looks up `helix_session_id` from `requestToSessionMapping[request_id]`
   - API stores `contextMappings[acp_thread_id] = helix_session_id`
   - **FIX**: API updates EXISTING Helix session (don't create new one)

5. Zed streams response back
   - Uses existing `contextMappings` lookup ✅
   - Updates SAME Helix session ✅
   - Publishes to correct user's pubsub ✅

## Implementation Checklist

### Helix API Changes
- [x] Add `requestToSessionMapping` to apiServer struct (already done)
- [ ] Add `externalAgentSessionMapping map[string]string` to apiServer struct
- [ ] Modify `/api/v1/external-agents` handler:
  - [ ] Create Helix session FIRST (before starting Wolf app)
  - [ ] Store externalAgentSessionMapping[agent_session_id] = helix_session_id
  - [ ] Generate request_id, store requestToSessionMapping[request_id] = helix_session_id
  - [ ] After Zed connects, send initial chat_message with request_id
- [ ] Update `handleThreadCreated` to check `requestToSessionMapping` FIRST
  - [ ] If request_id found → use that helix_session_id (don't create new)
  - [ ] If not found → fall back to current logic (create new session)
- [ ] Clean up requestToSessionMapping after thread is mapped

### Test Script Changes
- [ ] Test should still call `/api/v1/external-agents` (keep existing flow)
- [ ] Verify Helix session is created with correct owner
- [ ] Verify thread_created maps to EXISTING session (not new one)
- [ ] Verify responses flow back to SAME session
- [ ] Verify frontend receives updates for correct user

### Zed Changes
- [ ] NONE - Zed already works correctly ✅
