# ⚠️ OUT OF DATE - DO NOT USE ⚠️

**THIS DOCUMENT IS OUTDATED AND KEPT FOR HISTORICAL REFERENCE ONLY**

**See the authoritative spec at:** `/home/luke/pm/zed/WEBSOCKET_PROTOCOL_SPEC.md`

---

# WebSocket Protocol Design: SpecTask & Session Scoped Agents

## Current Implementation Audit

### What's Actually Implemented

**Connection Flow:**
1. **Helix creates Wolf app** with environment: `ZED_HELIX_URL=api:8080`, `HELIX_SESSION_ID=ses_xxx` (for session-scoped)
2. **Zed launches** and reads `ZED_HELIX_URL` from environment
3. **Zed generates agent_id**: `zed-agent-{timestamp}` (NOT from environment)
4. **Zed connects**: `ws://api:8080/api/v1/external-agents/sync?agent_id=zed-agent-{timestamp}`
5. **Helix extracts**: `agent_id` from query param (or `session_id` after recent fix)
6. **Helix registers**: Connection under `agent_id` in `externalAgentWSManager.connections`

**Message Flow (Helix → Zed):**
```json
{
  "type": "create_thread",
  "data": {
    "session_id": "ses_01k6hx6ejfjgt65x07xeqederb",  // Helix Session ID
    "message": "User's prompt"
  }
}
```

**Message Flow (Zed → Helix):**
```json
{
  "session_id": "ses_01k6hx6ejfjgt65x07xeqederb",  // Helix Session ID (from incoming message)
  "event_type": "chat_response",
  "data": {
    "content": "AI response...",
    "request_id": "req_123"
  }
}
```

### Key Issues with Current Implementation

1. **Agent ID Mismatch**:
   - Helix knows `session_id` but Zed connects with random `agent_id`
   - No way to route messages to correct Zed instance
   - Currently broadcasts to ALL connected agents (inefficient)

2. **No Multi-Session Support**:
   - One Wolf app = one agent_id = intended for one session
   - No concept of spectask-scoped agents with multiple threads

3. **No Bidirectional Thread Mapping**:
   - Zed doesn't track which Zed thread maps to which Helix session
   - No way to switch Zed UI when user views different Helix session

---

## Proposed Architecture: Two Agent Scoping Models

### Model 1: Session-Scoped (Current, Improved)
**Use Case**: Agent-spawned sessions WITHOUT SpecTask
- One Wolf app per session
- Lifecycle: Destroyed when session ends
- Simple 1:1 mapping

### Model 2: SpecTask-Scoped (New)
**Use Case**: User-driven workflows with multiple phases
- One Wolf app per SpecTask
- Multiple sessions (planning, implementation, review) share same Zed instance
- Each session = one Zed thread
- Lifecycle: Destroyed when SpecTask completes

---

## Protocol Design

### 1. Connection Establishment

**Helix Creates Wolf App** (with environment variables):

**Session-Scoped:**
```bash
HELIX_SCOPE_TYPE=session
HELIX_SCOPE_ID=ses_01k6hx6ejfjgt65x07xeqederb
HELIX_AGENT_INSTANCE_ID=zed-session-ses_01k6hx6ejfjgt65x07xeqederb
ZED_HELIX_URL=api:8080
ZED_HELIX_TOKEN=auth_token_here
```

**SpecTask-Scoped:**
```bash
HELIX_SCOPE_TYPE=spectask
HELIX_SCOPE_ID=task_01k6hx9abc123xyz
HELIX_AGENT_INSTANCE_ID=zed-spectask-task_01k6hx9abc123xyz
ZED_HELIX_URL=api:8080
ZED_HELIX_TOKEN=auth_token_here
```

**Zed Reads Environment and Connects:**
```rust
let scope_type = env::var("HELIX_SCOPE_TYPE").unwrap_or("session".to_string());
let scope_id = env::var("HELIX_SCOPE_ID").unwrap_or_default();
let agent_instance_id = env::var("HELIX_AGENT_INSTANCE_ID").unwrap_or_else(|| {
    format!("zed-agent-{}", chrono::Utc::now().timestamp())
});

// Connect with agent_instance_id as query param
let url = format!("ws://api:8080/api/v1/external-agents/sync?agent_id={}", agent_instance_id);
```

**WebSocket URL:**
```
ws://api:8080/api/v1/external-agents/sync?agent_id=zed-spectask-task_01k6hx9abc123xyz
```

**Helix Extracts and Registers:**
```go
agentID := req.URL.Query().Get("agent_id")  // "zed-spectask-task_01k6hx9abc123xyz"

// Register connection with instance ID
wsConn := &ExternalAgentWSConnection{
    AgentInstanceID: agentID,
    ScopeType:       extractScopeType(agentID),  // "spectask" or "session"
    ScopeID:         extractScopeID(agentID),    // "task_01k6hx9abc123xyz"
    Conn:            conn,
    SendChan:        make(chan types.ExternalAgentCommand, 100),
}
```

### 2. Agent Registration Message (Optional Enhancement)

After WebSocket connects, Zed sends registration:

```json
{
  "type": "agent_register",
  "data": {
    "agent_instance_id": "zed-spectask-task_01k6hx9abc123xyz",
    "scope_type": "spectask",
    "scope_id": "task_01k6hx9abc123xyz",
    "capabilities": ["multiple_threads"],
    "zed_version": "0.123.0"
  }
}
```

### 3. Session Assignment & Thread Creation

**Helix Assigns Session to Agent:**

```go
// For SpecTask-scoped
if session.SpecTaskID != "" {
    agentInstanceID := fmt.Sprintf("zed-spectask-%s", session.SpecTaskID)
    agent := agentRegistry.GetAgent(agentInstanceID)

    // Track session → agent mapping
    agentRegistry.AssignSession(session.ID, agentInstanceID)
}

// For Session-scoped
else {
    agentInstanceID := fmt.Sprintf("zed-session-%s", session.ID)
    agent := agentRegistry.GetAgent(agentInstanceID)

    // Single session for this agent
    agentRegistry.AssignSession(session.ID, agentInstanceID)
}
```

**Helix → Zed (Create Thread):**
```json
{
  "type": "thread_create",
  "data": {
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb",
    "spectask_id": "task_01k6hx9abc123xyz",  // null for session-scoped
    "zed_thread_id": null,  // null = create new, or existing to resume
    "message": "Let's plan the feature implementation",
    "metadata": {
      "phase": "planning",
      "user_id": "user_123"
    }
  }
}
```

**Zed → Helix (Thread Created):**
```json
{
  "type": "thread_created",
  "data": {
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb",
    "zed_thread_id": "zed-context-uuid-1234",
    "spectask_id": "task_01k6hx9abc123xyz"
  }
}
```

### 4. Message Exchange

**Helix → Zed (User Message):**
```json
{
  "type": "chat_message",
  "data": {
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb",
    "zed_thread_id": "zed-context-uuid-1234",  // from mapping
    "message": "Continue with the implementation",
    "request_id": "req_456"
  }
}
```

**Zed → Helix (AI Response - Streaming):**
```json
{
  "type": "chat_response",
  "data": {
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb",
    "zed_thread_id": "zed-context-uuid-1234",
    "content": "I'll help you implement...",
    "request_id": "req_456"
  }
}
```

**Zed → Helix (Response Complete):**
```json
{
  "type": "chat_response_done",
  "data": {
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb",
    "zed_thread_id": "zed-context-uuid-1234",
    "request_id": "req_456"
  }
}
```

### 5. Thread Switching / Focus Events

**Zed → Helix (User switched threads in Zed UI):**
```json
{
  "type": "thread_focused",
  "data": {
    "zed_thread_id": "zed-context-uuid-1234",
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb"
  }
}
```

**Helix → Zed (User viewing different session in UI):**
```json
{
  "type": "thread_focus_request",
  "data": {
    "helix_session_id": "ses_01k6hx6ejfjgt65x07xeqederb",
    "zed_thread_id": "zed-context-uuid-1234"
  }
}
```

---

## Implementation Details

### Helix Side

#### 1. Agent Instance Registry

```go
type AgentScope string

const (
    ScopeSpecTask AgentScope = "spectask"
    ScopeSession  AgentScope = "session"
)

type AgentInstance struct {
    AgentInstanceID string                    // "zed-spectask-task_xxx" or "zed-session-ses_xxx"
    ScopeType       AgentScope
    ScopeID         string                    // spectask_id or session_id

    WebSocketConn   *ExternalAgentWSConnection
    WolfAppID       int64
    UserID          string

    // Thread mappings (for spectask-scoped agents with multiple sessions)
    ThreadMappings  map[string]string         // helix_session_id → zed_thread_id

    CreatedAt       time.Time
    LastActivity    time.Time
}

type AgentRegistry struct {
    mu              sync.RWMutex
    instances       map[string]*AgentInstance // agent_instance_id → instance
    sessionToAgent  map[string]string         // helix_session_id → agent_instance_id
    spectaskToAgent map[string]string         // spectask_id → agent_instance_id
}
```

#### 2. Session Assignment Logic

```go
func (r *AgentRegistry) AssignSession(session *types.Session) (*AgentInstance, error) {
    r.mu.Lock()
    defer r.mu.Unlock()

    var agent *AgentInstance
    var exists bool

    if session.SpecTaskID != "" {
        // SpecTask-scoped: reuse existing agent for this spectask
        agentInstanceID := fmt.Sprintf("zed-spectask-%s", session.SpecTaskID)
        agent, exists = r.instances[agentInstanceID]

        if !exists {
            return nil, fmt.Errorf("spectask agent not found: %s", agentInstanceID)
        }

        // Add session to this spectask agent's mappings
        agent.ThreadMappings[session.ID] = ""  // Will be filled when Zed responds

    } else {
        // Session-scoped: dedicated agent for this session
        agentInstanceID := fmt.Sprintf("zed-session-%s", session.ID)
        agent, exists = r.instances[agentInstanceID]

        if !exists {
            return nil, fmt.Errorf("session agent not found: %s", agentInstanceID)
        }

        // Single session for this agent
        agent.ThreadMappings[session.ID] = ""
    }

    // Track session → agent mapping
    r.sessionToAgent[session.ID] = agent.AgentInstanceID
    agent.LastActivity = time.Now()

    return agent, nil
}

func (r *AgentRegistry) GetAgentForSession(sessionID string) (*AgentInstance, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    agentID, exists := r.sessionToAgent[sessionID]
    if !exists {
        return nil, false
    }

    agent, exists := r.instances[agentID]
    return agent, exists
}
```

#### 3. Message Routing

```go
func (s *HelixAPIServer) sendSessionToWebSocketAgent(ctx context.Context, sessionID string, message string) error {
    // Get agent for this session
    agent, exists := s.agentRegistry.GetAgentForSession(sessionID)
    if !exists {
        return fmt.Errorf("no agent found for session %s", sessionID)
    }

    // Get Zed thread ID from mapping (if exists)
    zedThreadID := agent.ThreadMappings[sessionID]

    // Determine message type based on whether thread exists
    msgType := "chat_message"
    if zedThreadID == "" {
        msgType = "thread_create"
    }

    // Create command
    command := types.ExternalAgentCommand{
        Type: msgType,
        Data: map[string]interface{}{
            "helix_session_id": sessionID,
            "zed_thread_id":    zedThreadID,
            "spectask_id":      agent.ScopeType == ScopeSpecTask ? agent.ScopeID : nil,
            "message":          message,
        },
    }

    // Send to specific agent
    select {
    case agent.WebSocketConn.SendChan <- command:
        log.Info().
            Str("agent_id", agent.AgentInstanceID).
            Str("session_id", sessionID).
            Msg("Sent message to agent")
        return nil
    default:
        return fmt.Errorf("agent send channel full")
    }
}
```

#### 4. Wolf Executor Updates

```go
func (w *WolfExecutor) CreateSpecTaskAgent(ctx context.Context, spectaskID, userID string) (*AgentInstance, error) {
    agentInstanceID := fmt.Sprintf("zed-spectask-%s", spectaskID)
    wolfAppID := w.generateWolfAppID(userID, spectaskID)

    env := []string{
        fmt.Sprintf("HELIX_SCOPE_TYPE=spectask"),
        fmt.Sprintf("HELIX_SCOPE_ID=%s", spectaskID),
        fmt.Sprintf("HELIX_AGENT_INSTANCE_ID=%s", agentInstanceID),
        fmt.Sprintf("ZED_HELIX_URL=api:8080"),
        fmt.Sprintf("ZED_HELIX_TOKEN=%s", w.apiToken),
    }

    // Create Wolf app
    err := w.createWolfApp(ctx, wolfAppID, env, workspaceDir)
    if err != nil {
        return nil, err
    }

    agent := &AgentInstance{
        AgentInstanceID: agentInstanceID,
        ScopeType:       ScopeSpecTask,
        ScopeID:         spectaskID,
        ThreadMappings:  make(map[string]string),
        WolfAppID:       wolfAppID,
        UserID:          userID,
        CreatedAt:       time.Now(),
    }

    // Register in registry
    w.agentRegistry.RegisterAgent(agent)

    return agent, nil
}

func (w *WolfExecutor) CreateSessionAgent(ctx context.Context, sessionID, userID string) (*AgentInstance, error) {
    agentInstanceID := fmt.Sprintf("zed-session-%s", sessionID)
    wolfAppID := w.generateWolfAppID(userID, sessionID)

    env := []string{
        fmt.Sprintf("HELIX_SCOPE_TYPE=session"),
        fmt.Sprintf("HELIX_SCOPE_ID=%s", sessionID),
        fmt.Sprintf("HELIX_AGENT_INSTANCE_ID=%s", agentInstanceID),
        fmt.Sprintf("ZED_HELIX_URL=api:8080"),
        fmt.Sprintf("ZED_HELIX_TOKEN=%s", w.apiToken),
    }

    // Create Wolf app
    err := w.createWolfApp(ctx, wolfAppID, env, workspaceDir)
    if err != nil {
        return nil, err
    }

    agent := &AgentInstance{
        AgentInstanceID: agentInstanceID,
        ScopeType:       ScopeSession,
        ScopeID:         sessionID,
        ThreadMappings:  make(map[string]string),
        WolfAppID:       wolfAppID,
        UserID:          userID,
        CreatedAt:       time.Now(),
    }

    // Register in registry
    w.agentRegistry.RegisterAgent(agent)

    return agent, nil
}
```

### Zed Side

#### 1. Thread Mapping Store

```rust
pub struct ThreadMappingStore {
    helix_to_zed: HashMap<String, ContextId>,  // helix_session_id → zed context_id
    zed_to_helix: HashMap<ContextId, String>,  // zed context_id → helix_session_id
}

impl ThreadMappingStore {
    pub fn new() -> Self {
        Self {
            helix_to_zed: HashMap::new(),
            zed_to_helix: HashMap::new(),
        }
    }

    pub fn map(&mut self, helix_session_id: String, zed_context_id: ContextId) {
        self.helix_to_zed.insert(helix_session_id.clone(), zed_context_id);
        self.zed_to_helix.insert(zed_context_id, helix_session_id);
    }

    pub fn get_zed_thread(&self, helix_session_id: &str) -> Option<ContextId> {
        self.helix_to_zed.get(helix_session_id).cloned()
    }

    pub fn get_helix_session(&self, zed_context_id: &ContextId) -> Option<String> {
        self.zed_to_helix.get(zed_context_id).cloned()
    }

    pub fn remove(&mut self, helix_session_id: &str) {
        if let Some(context_id) = self.helix_to_zed.remove(helix_session_id) {
            self.zed_to_helix.remove(&context_id);
        }
    }
}
```

#### 2. Connection with Scope

```rust
// Read scope from environment
let scope_type = env::var("HELIX_SCOPE_TYPE").unwrap_or("session".to_string());
let scope_id = env::var("HELIX_SCOPE_ID").unwrap_or_default();
let agent_instance_id = env::var("HELIX_AGENT_INSTANCE_ID").unwrap_or_else(|| {
    format!("zed-agent-{}", chrono::Utc::now().timestamp())
});

// Connect with agent_instance_id
let websocket_config = WebSocketSyncConfig {
    enabled: true,
    helix_url: helix_url.clone(),
    session_id: agent_instance_id.clone(),  // Used as agent_id in URL
    auth_token: auth_token.clone(),
    use_tls: false,
};
```

#### 3. Thread Creation Handler

```rust
fn handle_thread_create(&mut self, msg: ThreadCreateMessage, window: &mut Window, cx: &mut WindowContext) {
    // Check if thread already exists for this session
    let context_id = if let Some(existing_thread) = self.thread_mappings.get_zed_thread(&msg.helix_session_id) {
        // Resume existing thread
        existing_thread
    } else {
        // Create new thread
        let context_id = self.agent_panel.new_prompt_editor_with_message(window, cx, &msg.message);

        // Map Helix session → Zed thread
        self.thread_mappings.map(msg.helix_session_id.clone(), context_id);

        // Notify Helix of the mapping
        self.send_thread_created(msg.helix_session_id, context_id, msg.spectask_id);

        context_id
    };

    // Focus the thread in UI
    self.agent_panel.focus_thread(context_id, window, cx);
}

fn send_thread_created(&self, helix_session_id: String, context_id: ContextId, spectask_id: Option<String>) {
    let message = json!({
        "type": "thread_created",
        "data": {
            "helix_session_id": helix_session_id,
            "zed_thread_id": context_id.to_string(),
            "spectask_id": spectask_id,
        }
    });

    self.websocket_sync.send_message(message);
}
```

#### 4. Thread Focus Handler

```rust
// When user switches threads in Zed
fn on_thread_focused(&mut self, context_id: ContextId, cx: &mut WindowContext) {
    if let Some(helix_session_id) = self.thread_mappings.get_helix_session(&context_id) {
        let message = json!({
            "type": "thread_focused",
            "data": {
                "zed_thread_id": context_id.to_string(),
                "helix_session_id": helix_session_id,
            }
        });

        self.websocket_sync.send_message(message);
    }
}

// When Helix requests focus on specific thread
fn handle_thread_focus_request(&mut self, msg: ThreadFocusMessage, window: &mut Window, cx: &mut WindowContext) {
    if let Some(context_id) = self.thread_mappings.get_zed_thread(&msg.helix_session_id) {
        self.agent_panel.focus_thread(context_id, window, cx);
    }
}
```

---

## Example Flows

### Flow 1: SpecTask with Multiple Sessions (Reusable Container)

```
1. User creates SpecTask "task_123" for "Build Authentication System"
2. Helix creates spectask-scoped agent:
   - WolfExecutor.CreateSpecTaskAgent(spectaskID: "task_123")
   - Wolf app created with env: HELIX_SCOPE_TYPE=spectask, HELIX_SCOPE_ID=task_123
   - Agent instance ID: "zed-spectask-task_123"

3. Zed launches and connects:
   - Reads env: HELIX_AGENT_INSTANCE_ID=zed-spectask-task_123
   - Connects: ws://api:8080/api/v1/external-agents/sync?agent_id=zed-spectask-task_123
   - Helix registers agent instance

4. User starts "Planning" session (ses_001):
   - Session created with spectask_id="task_123"
   - Helix assigns session to agent: agentRegistry.AssignSession(ses_001)
   - Helix → Zed: thread_create {helix_session_id: "ses_001", spectask_id: "task_123"}
   - Zed creates thread "zed-ctx-001", maps ses_001 → zed-ctx-001
   - Zed → Helix: thread_created {helix_session_id: "ses_001", zed_thread_id: "zed-ctx-001"}

5. User completes planning, starts "Implementation" session (ses_002):
   - Session created with spectask_id="task_123" (SAME spectask)
   - Helix assigns to SAME agent (zed-spectask-task_123)
   - Helix → Zed: thread_create {helix_session_id: "ses_002", spectask_id: "task_123"}
   - Zed creates NEW thread "zed-ctx-002" in SAME container
   - Maps ses_002 → zed-ctx-002
   - Both threads now visible in Zed UI

6. User switches between sessions in Helix:
   - Views ses_001 → Helix → Zed: thread_focus_request {helix_session_id: "ses_001"}
   - Zed switches UI to thread zed-ctx-001

7. SpecTask completes:
   - Helix calls WolfExecutor.CleanupSpecTaskAgent("task_123")
   - Wolf app destroyed, both sessions archived
```

### Flow 2: Session-Scoped (Agent-Spawned, No SpecTask)

```
1. AI agent spawns session ses_456 (NO spectask_id)
2. Helix creates session-scoped agent:
   - WolfExecutor.CreateSessionAgent(sessionID: "ses_456")
   - Wolf app created with env: HELIX_SCOPE_TYPE=session, HELIX_SCOPE_ID=ses_456
   - Agent instance ID: "zed-session-ses_456"

3. Zed launches and connects:
   - Reads env: HELIX_AGENT_INSTANCE_ID=zed-session-ses_456
   - Connects: ws://api:8080/api/v1/external-agents/sync?agent_id=zed-session-ses_456
   - Helix registers agent instance

4. Session messages:
   - Helix → Zed: thread_create {helix_session_id: "ses_456", spectask_id: null}
   - Zed creates thread "zed-ctx-456", maps ses_456 → zed-ctx-456
   - Single thread for this agent (session-scoped)

5. Session completes:
   - Helix calls WolfExecutor.CleanupSessionAgent("ses_456")
   - Wolf app destroyed immediately
```

---

## Migration Strategy

### Phase 1: Fix Current Session-Scoped Implementation (Immediate)
1. ✅ Update `handleExternalAgentSync` to extract `session_id` from query
2. Add `HELIX_AGENT_INSTANCE_ID` to Wolf app environment
3. Update Zed to read `HELIX_AGENT_INSTANCE_ID` instead of generating random ID
4. Test single-session flow works correctly

### Phase 2: Add Agent Registry (Foundation)
1. Implement `AgentRegistry` in Helix
2. Track agent instances and session assignments
3. Update Wolf executor to register agents
4. Update message routing to use registry

### Phase 3: Add SpecTask Support
1. Add `spectask_id` field to session model
2. Implement `CreateSpecTaskAgent` in Wolf executor
3. Add spectask → agent mapping in registry
4. Update session assignment logic to reuse spectask agents

### Phase 4: Thread Mapping & Focus
1. Implement `ThreadMappingStore` in Zed
2. Add `thread_created` response message
3. Implement thread focus events (both directions)
4. Update UI to handle thread switching

### Phase 5: Lifecycle Management
1. Implement spectask lifecycle hooks
2. Add agent cleanup on spectask completion
3. Add session cleanup on session end
4. Handle reconnection scenarios

---

## Open Questions

1. **Authentication**: How to secure agent-specific auth tokens?
2. **Reconnection**: What happens if WebSocket disconnects mid-spectask?
3. **Thread Persistence**: Should spectask threads persist across agent restarts?
4. **Multi-User**: Can multiple users share a spectask agent? (Probably not initially)
5. **Resource Limits**: Max threads per spectask agent? Max sessions per agent?

---

## Success Criteria

- [ ] Session-scoped agents work reliably (1:1 session:agent)
- [ ] SpecTask-scoped agents support multiple sessions (N:1 sessions:agent)
- [ ] Messages route to correct agent instance
- [ ] Thread mappings maintained bidirectionally
- [ ] UI switches threads when user changes sessions
- [ ] Lifecycle cleanup works correctly for both scope types
- [ ] No message loss during thread creation/switching
- [ ] Performance acceptable with 10+ threads per spectask agent
