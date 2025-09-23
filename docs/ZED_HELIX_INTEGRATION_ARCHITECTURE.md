# Zed-Helix Integration Architecture

## Overview

This document describes the architecture for integrating Zed (the code editor) with Helix as an external AI agent. The integration enables Zed to act as both a "source of truth" for conversation threads and as an external agent that can be invoked by Helix sessions.

## Key Architectural Principles

### 1. **Separation of Concerns: Lifecycle vs Session Communication**

**CRITICAL DISTINCTION:**
- **NATS**: Used ONLY for Zed instance lifecycle management (start/stop instances)
- **WebSocket**: Used for ALL session communication (messages, responses, thread sync)

```
┌─────────────────┐    NATS (Lifecycle)     ┌──────────────────┐
│ Helix Controller├────────────────────────►│ Zed Agent Runner │
│                 │    (start/stop)         │                  │
└─────────────────┘                         └──────────────────┘
                                                      │
                                                      │ spawns
                                                      ▼
┌─────────────────┐   WebSocket (Sessions)   ┌──────────────────┐
│ Helix WebSocket ├◄──────────────────────►│ Zed Instance     │
│ Manager         │   (messages/threads)     │ (AI Assistant)   │
└─────────────────┘                         └──────────────────┘
```

### 2. **Zed as Source of Truth**

- **Zed maintains the complete conversation state** (AssistantContext)
- **Helix acts as a message gateway** - pushes user prompts to Zed
- **Zed responds with complete thread state** back to Helix
- **Helix reflects Zed's state** in its UI/API

### 3. **Unified WebSocket Communication**

**Both Helix-managed and external Zed instances use the same WebSocket path:**
- Helix-managed: Spawned via NATS → Connects to WebSocket
- External: User's desktop → Connects directly to WebSocket

## Architecture Components

### Helix Side

#### 1. **Session Handlers** (`/api/pkg/server/session_handlers.go`)
- **Routes external agent sessions** to WebSocket (NOT NATS)
- **Determines agent type** from app configuration
- **Handles session name generation** (skipped for external agents)

#### 2. **External Agent WebSocket Manager** (`/api/pkg/server/websocket_external_agent_sync.go`)
- **Manages WebSocket connections** from Zed instances
- **Endpoint**: `/api/v1/external-agents/sync`
- **Handles bidirectional communication** between Helix sessions and Zed threads

#### 3. **Agent Session Manager** (`/api/pkg/controller/agent_session_manager.go`)
- **ISSUE**: Currently routes session requests to NATS
- **NEEDS FIX**: Should route to WebSocket for session communication

### Zed Side

#### 1. **External WebSocket Sync** (`crates/external_websocket_sync/`)
- **Connects to Helix WebSocket** endpoint
- **Handles thread creation requests** from Helix
- **Publishes context changes** back to Helix

#### 2. **Agent Panel Integration** (`crates/agent_ui/src/agent_panel.rs`)
- **Creates real AssistantContext** instances from WebSocket requests
- **Triggers AI completion** using configured LLM providers
- **Emits context change events** for sync back to Helix

## Message Flow

### 1. **Helix → Zed (New User Message)**

```
User sends message to Helix session
         ↓
Helix session handler (agent_type: "zed_external")
         ↓
WebSocket manager sends to connected Zed instance
         ↓
Zed receives thread creation request
         ↓
Zed creates AssistantContext with user message
         ↓
Zed triggers AI completion (Anthropic/etc.)
```

### 2. **Zed → Helix (AI Response Sync)**

```
Zed AssistantContext emits change event
         ↓
External WebSocket Sync captures event
         ↓
Sync service extracts complete thread state
         ↓
WebSocket sends thread state back to Helix
         ↓
Helix updates session with complete conversation
```

## Current Status & Issues

### ✅ **Working Components**

1. **Zed WebSocket Connection**: Successfully connects to Helix WebSocket endpoint
2. **WebSocket Routing**: Helix correctly routes external agent sessions via WebSocket (not NATS)
3. **Agent Type Detection**: Helix correctly identifies `zed_external` from app config
4. **External Agent Config**: Proper handling of empty/nil configurations
5. **Security**: API keys properly loaded from .env file, no hardcoded secrets

### ❌ **CRITICAL ARCHITECTURAL GAPS**

After extensive debugging, the integration is **fundamentally broken** due to these issues:

#### 1. **Session Creation Flow is Wrong**
**Problem**: Helix expects external agents to respond to session creation requests, but Zed never does.
```
Helix: "Create session with external agent" → WebSocket → Zed
Zed: (receives message, creates thread, but NEVER responds back)
Helix: (times out waiting for response, never creates session)
Result: No Helix session = No URL to view = No way to send messages
```

#### 2. **Zed UI Integration Missing**
**Problem**: Zed WebSocket sync creates AssistantContext but doesn't integrate with UI
- AI panel doesn't open automatically
- Threads don't appear in UI
- No visual indication that external messages arrived
- AssistantContext exists in memory but not in UI

#### 3. **Message Flow Architecture Mismatch**
**Current Flow (Broken)**:
```
1. Helix tries to create session → Sends WebSocket message → Zed
2. Zed receives message → Creates thread → STOPS HERE
3. Helix times out → No session created → Dead end
```

**Needed Flow**:
```
1. Helix creates session immediately (no external agent dependency)
2. Helix sends message → WebSocket → Zed  
3. Zed creates thread → Shows in UI → Triggers AI
4. Zed sends AI response → WebSocket → Helix
5. Helix updates session with response
```

#### 4. **Agent Readiness Protocol Missing**
**Problem**: No handshake protocol for external agent availability
- Helix doesn't know if external agents are ready
- No way to distinguish "agent busy" vs "agent available"
- No graceful fallback if external agent unavailable

#### 5. **Bidirectional Sync Incomplete**
**Problem**: Only Helix → Zed works, Zed → Helix is missing
- Zed creates threads but never sends responses back
- No event listeners for AssistantContext changes
- No mechanism to sync AI responses back to Helix

### ❌ **SPECIFIC TECHNICAL ISSUES**

1. **Zed Agent Panel Integration**: WebSocket sync bypasses normal UI flow
2. **Context Event Handling**: No listeners for AI response completion
3. **Session Lifecycle**: Helix session creation depends on external agent response
4. **Message Serialization**: Complex message content objects not properly handled
5. **Thread Persistence**: Threads created in memory but not persisted to Zed's database

### ✅ **Architecture Fix Implemented**

**Problem**: Session communication was going through NATS ❌
```go
// WRONG: This sent session requests to NATS
err = s.Controller.LaunchExternalAgent(req.Context(), session.ID, "zed")
```

**Solution**: Session communication now goes through WebSocket ✅
```go
// RIGHT: This sends directly to WebSocket manager
err = s.sendSessionToWebSocketAgents(req.Context(), session.ID, lastMessage.Content)
```

**Implementation**: 
- Modified `session_handlers.go` to bypass controller and NATS
- Added `sendSessionToWebSocketAgents()` function
- Routes session requests directly to connected WebSocket agents
- Extracts message content from complex objects

## Architectural Solutions Required

### **SOLUTION 1: Fix Session Creation Flow**

**Change Helix behavior**: Don't wait for external agent response during session creation
```go
// CURRENT (Broken): Wait for external agent response
if agentType == "zed_external" {
    // Send to WebSocket and WAIT for response
    err = waitForExternalAgentResponse(sessionID)  // ❌ This times out
}

// FIXED: Create session immediately, send message separately  
if agentType == "zed_external" {
    // Create session immediately
    session := createSessionNow()
    // Send initial message async (don't wait for response)
    go sendToExternalAgentAsync(session.ID, initialMessage)
}
```

### **SOLUTION 2: Fix Zed UI Integration**

**Problem**: WebSocket sync creates threads but doesn't show them in UI
**Solution**: Integrate WebSocket sync with AgentPanel properly

```rust
// CURRENT: Creates context but doesn't show in UI
let context = self.context_store.create(cx);  // ❌ Hidden

// NEEDED: Create context AND show in UI
let context = self.context_store.create(cx);
self.agent_panel.show_context(context);       // ✅ Visible
self.agent_panel.open_ai_panel();             // ✅ Panel opens
```

### **SOLUTION 3: Implement Agent Readiness Protocol**

**Add handshake protocol**:
```
1. Zed connects → Sends "agent_ready" message
2. Helix marks agent as available
3. Session creation checks agent availability
4. If available: route to agent, if not: fallback to internal
```

### **SOLUTION 4: Complete Bidirectional Sync**

**Add response flow from Zed → Helix**:
```rust
// Listen for AI response completion
context.on_response_complete(|response| {
    let sync_message = SyncMessage {
        session_id: external_session_id,
        event_type: "ai_response",
        data: response.content,
    };
    websocket_sync.send_to_helix(sync_message);
});
```

## Implementation Plan (Revised)

### **Phase 1: Fix Session Creation (Critical)**
- [ ] Remove external agent dependency from session creation
- [ ] Create sessions immediately for external agents  
- [ ] Send initial messages asynchronously after session creation
- [ ] Add proper error handling for agent unavailable

### **Phase 2: Fix Zed UI Integration (Critical)**
- [ ] Make WebSocket-created threads visible in Agent Panel
- [ ] Auto-open AI panel when external messages arrive
- [ ] Ensure threads persist to Zed's database
- [ ] Add visual indicators for external agent activity

### **Phase 3: Complete Response Flow**
- [ ] Add event listeners for AssistantContext changes in Zed
- [ ] Implement Zed → Helix response sync via WebSocket
- [ ] Handle AI response streaming back to Helix
- [ ] Update Helix sessions with complete conversation state

### **Phase 4: Add Agent Readiness Protocol**
- [ ] Implement agent registration/heartbeat system
- [ ] Add agent availability checking in Helix
- [ ] Graceful fallback when external agents unavailable
- [ ] Agent status monitoring and reconnection logic

## Key Files

### Helix
- `api/pkg/controller/agent_session_manager.go` - Session routing (NEEDS FIX)
- `api/pkg/server/websocket_external_agent_sync.go` - WebSocket management
- `api/pkg/server/session_handlers.go` - Session creation and agent type detection

### Zed  
- `crates/external_websocket_sync/` - WebSocket client and sync logic
- `crates/agent_ui/src/agent_panel.rs` - AI assistant integration

## Testing

### Integration Test
- `helix/integration-test/zed-websocket/enhanced_source_of_truth.go`
- Tests full bidirectional sync with Anthropic API
- Validates "Zed as source of truth" architecture

## Lessons Learned

1. **Don't mix lifecycle and session communication** - they serve different purposes
2. **WebSocket is the universal session interface** - both managed and external instances use it
3. **NATS is only for runner management** - starting/stopping Zed processes
4. **App configuration drives agent type** - don't rely on request parameters
5. **External agent configs can be empty** - handle gracefully with defaults

---

*Last updated: September 2025*
*Status: Architecture documented, implementation in progress*
