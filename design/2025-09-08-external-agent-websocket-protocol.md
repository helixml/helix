# ⚠️ OUT OF DATE - DO NOT USE ⚠️

**THIS DOCUMENT IS OUTDATED AND KEPT FOR HISTORICAL REFERENCE ONLY**

**See the authoritative spec at:** `/home/luke/pm/zed/WEBSOCKET_PROTOCOL_SPEC.md`

**For Helix implementation updates needed:** See `/home/luke/pm/helix/HELIX_PROTOCOL_UPDATE_NEEDED.md`

---

# External Agent WebSocket Protocol

## Core Concepts

### Helix Side
- **Session**: A conversation thread (e.g., `ses_01k6jg...`)
- **Interaction**: A single user request + AI response pair within a session
- **Interaction States**: `waiting` → `complete` or `error`

### Zed Side  
- **Context**: A conversation thread with the AI assistant (e.g., `8405cd2a-24ae-...`)
- **Message**: Individual messages within a context (user or assistant)
- **Context is the source of truth** for the conversation

### Key Mapping
- **One Helix Session ↔ One Zed Context** (1:1 relationship)
- **Only Helix maintains the mapping**: `helix_session_id → zed_context_id`
- Zed is stateless - doesn't maintain any mapping
- **All messages include BOTH IDs** (whichever are known at the time)

---

## Flow A: User Creates New Session in Helix

### Scenario
User opens Helix, creates a new session, sends first message to Zed agent.

### Message Flow

```
1. Helix → Zed: chat_message
{
  "command_type": "chat_message",
  "data": {
    "helix_session_id": "ses_01k6abc...",
    "zed_context_id": null,  // First message - no context yet
    "message": "Hello, can you help me?",
    "request_id": "req_1234567890"
  }
}

2. Zed receives chat_message
   - Sees zed_context_id is null → creates new context
   - Creates Zed context with UUID: "8405cd2a-24ae-..."
   - No mapping stored (Zed is stateless)
   - Adds user message to the context
   - Starts AI completion

3. Zed → Helix: context_created (ONLY ONCE, when real context is created)
{
  "session_id": "ses_01k6abc...",  // Helix session ID (echo back)
  "event_type": "context_created",
  "data": {
    "zed_context_id": "8405cd2a-24ae-...",
    "helix_session_id": "ses_01k6abc..."
  }
}

4. Helix receives context_created
   - Stores mapping: session["ses_01k6abc..."].zed_context_id = "8405cd2a-24ae-..."
   - Does NOT create new session (already exists)
   - Does NOT mark interaction complete yet

5. Zed → Helix: message_added (streaming AI response)
{
  "session_id": "ses_01k6abc...",  // Helix session ID
  "event_type": "message_added",
  "data": {
    "zed_context_id": "8405cd2a-24ae-...",
    "message_id": "ai_msg_1759410084",
    "role": "assistant",
    "content": "Hello! How can I",  // Partial content
    "timestamp": 1759410084
  }
}

6. Zed → Helix: message_added (continues streaming)
{
  "session_id": "ses_01k6abc...",
  "event_type": "message_added", 
  "data": {
    "zed_context_id": "8405cd2a-24ae-...",
    "message_id": "ai_msg_1759410084",  // SAME message_id
    "role": "assistant",
    "content": "Hello! How can I help you today?",  // Full content
    "timestamp": 1759410085
  }
}

7. Zed → Helix: message_completed
{
  "session_id": "ses_01k6abc...",
  "event_type": "message_completed",
  "data": {
    "zed_context_id": "8405cd2a-24ae-...",
    "message_id": "ai_msg_1759410084",
    "request_id": "req_1234567890"
  }
}

8. Helix receives message_completed
   - Finds the most recent waiting interaction for session "ses_01k6abc..."
   - Marks interaction as complete
   - Response from last message_added is already stored
```

---

## Flow B: Zed Agent Replies (Streaming)

### Key Points
- Zed sends `message_added` events as content arrives (streaming)
- **Same message_id** with progressively longer content
- Helix updates the interaction response on each `message_added`
- Only mark interaction complete when `message_completed` arrives

### Helix Behavior
```python
# Pseudocode for handling message_added
def handle_message_added(event):
    helix_session_id = event.session_id
    content = event.data.content
    
    # Find the waiting interaction for this session
    interaction = find_most_recent_waiting_interaction(helix_session_id)
    
    if interaction:
        # Update response (don't mark complete yet)
        interaction.response_message = content
        interaction.state = "waiting"  # Still waiting for completion
        save_interaction(interaction)
```

```python
# Pseudocode for handling message_completed  
def handle_message_completed(event):
    helix_session_id = event.session_id
    
    # Find the waiting interaction
    interaction = find_most_recent_waiting_interaction(helix_session_id)
    
    if interaction:
        # NOW mark it complete
        interaction.state = "complete"
        interaction.completed = now()
        save_interaction(interaction)
```

---

## Flow C: Follow-up Messages in Same Session

### Scenario
User sends another message in the same Helix session.

### Message Flow

```
1. Helix → Zed: chat_message
{
  "command_type": "chat_message",
  "data": {
    "helix_session_id": "ses_01k6abc...",  // SAME session
    "zed_context_id": "8405cd2a-24ae-...",  // Helix provides the context ID
    "message": "Can you explain more?",
    "request_id": "req_9876543210"
  }
}

2. Zed receives chat_message
   - Sees zed_context_id is provided → uses existing context
   - Finds existing context: "8405cd2a-24ae-..."
   - Adds user message to EXISTING context
   - Starts AI completion
   - Does NOT send context_created (already exists)

3. Zed → Helix: message_added (streaming)
{
  "session_id": "ses_01k6abc...",
  "event_type": "message_added",
  "data": {
    "zed_context_id": "8405cd2a-24ae-...",  // SAME context
    "message_id": "ai_msg_1759420000",  // NEW message
    "role": "assistant",
    "content": "Sure! Let me explain...",
    "timestamp": 1759420000
  }
}

4. Zed → Helix: message_completed
{
  "session_id": "ses_01k6abc...",
  "event_type": "message_completed",
  "data": {
    "zed_context_id": "8405cd2a-24ae-...",
    "message_id": "ai_msg_1759420000",
    "request_id": "req_9876543210"
  }
}
```

---

## Critical Implementation Rules

### Zed Side

1. **Zed is stateless** - no mapping to maintain

2. **Check zed_context_id in incoming message**:
   ```rust
   if let Some(zed_context_id) = message.data.get("zed_context_id") {
       // Use existing context
       add_message_to_context(zed_context_id, user_message);
   } else {
       // Create new context
       let context_id = create_new_context();
       send_context_created_event(helix_session_id, context_id);
   }
   ```

3. **Don't send synthetic context_created**:
   - Only send `context_created` when Zed UI actually creates a context
   - Include both `zed_context_id` and `helix_session_id` in the event

4. **Stream message_added with same message_id**:
   - As content arrives, send `message_added` with progressively longer content
   - Keep `message_id` the same for the same assistant message

5. **Send message_completed when AI is done**:
   - After AI stops generating, send `message_completed`
   - Include `request_id` so Helix knows which request finished

### Helix Side

1. **Store zed_context_id from context_created**:
   ```go
   // Store on the session object itself
   session.ZedContextID = zed_context_id
   UpdateSession(session)
   ```

1a. **When sending chat_message, include zed_context_id**:
   ```go
   command := ExternalAgentCommand{
       Type: "chat_message",
       Data: {
           "helix_session_id": session.ID,
           "zed_context_id": session.ZedContextID,  // null on first message
           "message": userMessage,
           "request_id": requestID,
       },
   }
   ```

2. **On message_added**:
   - Extract helix_session_id from event (it's in session_id field)
   - Get session by helix_session_id
   - Find most recent waiting interaction in that session
   - Update interaction.response_message with content
   - Keep state as `waiting` (don't mark complete yet)

3. **On message_completed**:
   - Extract helix_session_id from event (it's in session_id field)
   - Get session by helix_session_id
   - Find most recent waiting interaction in that session
   - Mark interaction.state = `complete`
   - Set interaction.completed timestamp

4. **Never mark interaction complete before message_completed arrives**

---

## Why This Works

### A) New Session
- Helix creates session and interaction in `waiting` state
- Zed creates new context and sends `context_created` ONCE
- Helix stores the mapping
- AI response streams via `message_added` events
- Helix updates response content but keeps `waiting` state
- When `message_completed` arrives, Helix marks interaction `complete`

### B) Streaming Responses
- Multiple `message_added` events with progressively longer content
- Helix updates interaction response on each event
- Only `message_completed` transitions interaction to `complete` state
- User sees response building up in real-time

### C) Follow-up Messages
- Helix sends `chat_message` with same `helix_session_id`
- Zed looks up mapping, finds existing `zed_context_id`
- Zed adds message to existing context (no new context created)
- Response flows same as (B)
- All messages stay in same Zed thread

---

## Event Summary

### Helix → Zed
- `chat_message`: Send user message (includes `helix_session_id`, `message`, `request_id`)

### Zed → Helix
- `context_created`: New Zed context created (includes `zed_context_id`, `helix_session_id`)
- `message_added`: AI response content (can be sent multiple times for streaming)
- `message_completed`: AI finished responding (includes `message_id`, `request_id`)
