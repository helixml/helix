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

1. **Zed WebSocket Connection**: Successfully connects to Helix
2. **Thread Creation**: Helix can create threads in Zed
3. **AI Integration**: Zed processes messages with real AI (Anthropic)
4. **Agent Type Detection**: Helix correctly identifies `zed_external` from app config
5. **External Agent Config**: Proper handling of empty/nil configurations

### ❌ **Current Issues**

1. **CRITICAL - Session Routing**: `agent_session_manager.go` routes to NATS instead of WebSocket
   - **Impact**: Integration test CANNOT work - gets "no consumers available" error
   - **Root Cause**: Session requests go to `pubsub.ZedAgentQueue` (NATS) instead of WebSocket
   - **Status**: BLOCKING - must fix before any session communication works

2. **Response Sync**: Zed → Helix response flow not fully implemented
3. **Message Content**: Complex message content objects need proper extraction

### 🔄 **Architecture Fix Needed**

**Problem**: Session communication goes through NATS
```go
// WRONG: This sends session requests to NATS
response, err := c.Options.PubSub.StreamRequest(
    ctx,
    pubsub.ZedAgentRunnerStream,
    pubsub.ZedAgentQueue,  // ← NATS queue
    data,
    header,
    30*time.Second,
)
```

**Solution**: Session communication should go through WebSocket
```go
// RIGHT: This should send to WebSocket manager
err := c.sendToWebSocketAgents(ctx, sessionID, messageData)
```

## Implementation Plan

### Phase 1: Fix Session Routing
- [ ] Modify `agent_session_manager.go` to route sessions via WebSocket
- [ ] Add WebSocket routing logic for external agents
- [ ] Ensure both lifecycle (NATS) and session (WebSocket) paths work correctly

### Phase 2: Complete Response Flow  
- [ ] Implement full Zed → Helix sync in `external_websocket_sync`
- [ ] Handle AssistantContext events and extract complete thread state
- [ ] Update Helix sessions with Zed's conversation state

### Phase 3: Production Readiness
- [ ] Add proper error handling and reconnection logic
- [ ] Implement authentication and authorization
- [ ] Add monitoring and observability

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
