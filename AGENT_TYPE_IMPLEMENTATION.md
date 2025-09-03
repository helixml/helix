# Agent Type Implementation

This document describes the implementation of agent type selection in Helix, allowing users to choose between **Helix Built-In Agents** and **Zed External Agents** for their sessions.

## Overview

The agent type system enables users to select the appropriate agent for their needs:

- **Helix Built-In Agent**: Traditional conversational AI with skills and tools
- **Zed External Agent**: Full development environment with code editing via remote desktop

## Architecture

### Core Pattern: Runner Pool with Container Isolation

We follow the same pattern as the previous gptscript runner, but with Zed agents:

```
Frontend → API → WebSocket → Runner Pool → Zed Containers
                     ↓
               Pub/Sub Queue
```

### Key Design Decisions

1. **Pool of Zed Runner Containers**: Scaled with `docker-compose up --scale zed-runner=N`
2. **One Session Per Container**: Each container handles one session then exits for cleanup
3. **Container Restart for Cleanup**: Natural cleanup between sessions, no persistent state
4. **RDP Proxy via WebSocket**: Frontend connects via API proxy, not direct RDP
5. **Ephemeral Container Filesystem**: No volumes, filesystem cleaned on container exit

## Implementation Details

### Backend Changes

#### 1. Agent Type Selection in Session Creation

```go
// api/pkg/types/types.go
type SessionChatRequest struct {
    // ... existing fields
    AgentType           string               `json:"agent_type"`
    ExternalAgentConfig *ExternalAgentConfig `json:"external_agent_config,omitempty"`
}

type ExternalAgentConfig struct {
    WorkspaceDir   string   `json:"workspace_dir,omitempty"`
    ProjectPath    string   `json:"project_path,omitempty"`
    EnvVars        []string `json:"env_vars,omitempty"`
    AutoConnectRDP bool     `json:"auto_connect_rdp,omitempty"`
}
```

#### 2. Session Routing Logic

```go
// api/pkg/server/session_handlers.go
if startReq.AgentType == "" {
    startReq.AgentType = "helix" // Default to Helix agent
}

if newSession && startReq.AgentType == "zed_external" {
    err = s.Controller.LaunchExternalAgent(req.Context(), session.ID, "zed")
    if err != nil {
        // Clean up session and return error
        s.Controller.DeleteSession(req.Context(), session.ID)
        http.Error(rw, "failed to launch external agent: "+err.Error(), http.StatusInternalServerError)
        return
    }
}
```

#### 3. Runner Pool Pattern

```go
// api/pkg/controller/agent_session_manager.go
func (c *Controller) launchZedAgent(ctx context.Context, sessionID string) error {
    // Create Zed agent request
    zedAgent := &types.ZedAgent{
        SessionID: sessionID,
        UserID:    session.Owner,
        Input:     "Initialize Zed development environment",
        // ... configuration from session metadata
    }

    // Dispatch to runner pool via pub/sub
    _, err = c.Options.PubSub.StreamRequest(
        ctx,
        pubsub.ZedAgentRunnerStream,
        pubsub.ZedAgentQueue,
        data,
        header,
        30*time.Second,
    )
}
```

#### 4. Zed Runner Implementation

```go
// api/pkg/zedagent/zed_runner.go (renamed from gptscript)
type ZedRunner struct {
    cfg         *config.GPTScriptRunnerConfig
    zedExecutor *external_agent.ZedExecutor
}

func (r *ZedRunner) Run(ctx context.Context) error {
    // Connect to API via WebSocket
    conn, err := r.dial(ctx)
    
    // Process one task then exit (container restarts for cleanup)
    if r.cfg.MaxTasks > 0 && ops.Load() >= uint64(r.cfg.MaxTasks) {
        log.Info().Msg("Zed runner completed task, exiting for container restart")
        cancel()
    }
}
```

### Frontend Changes

#### 1. Agent Type Selector Component

```typescript
// frontend/src/components/agent/AgentTypeSelector.tsx
<AgentTypeSelector
  value={agentType}
  onChange={(agentType, config) => {
    onSetSessionConfig(prevConfig => ({
      ...prevConfig,
      agentType,
      externalAgentConfig: config,
    }))
  }}
  externalAgentConfig={sessionConfig.externalAgentConfig}
  showExternalConfig={true}
/>
```

#### 2. Session Configuration

```typescript
// frontend/src/types.ts
export interface ICreateSessionConfig {
  // ... existing fields
  agentType: IAgentType,
  externalAgentConfig?: IExternalAgentConfig,
}
```

#### 3. Real RDP Client

```typescript
// frontend/src/components/external-agent/RDPViewer.tsx
// Uses Apache Guacamole protocol over WebSocket proxy
const initializeConnection = async (connInfo: RDPConnectionInfo) => {
    const wsUrl = connInfo.proxy_url;
    const ws = new WebSocket(wsUrl);
    
    // Implement Guacamole RDP protocol
    ws.onmessage = (event) => {
        const instructions = event.data.split(';').filter(Boolean);
        instructions.forEach(instructionStr => {
            const instruction = parseInstruction(instructionStr);
            handleDisplayInstruction(instruction); // Render to canvas
        });
    };
}
```

## Security Implementation

### 1. Secure Random Passwords

```go
// api/pkg/external-agent/zed_executor.go
func generateSecurePassword() (string, error) {
    const passwordLength = 24
    const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
    
    password := make([]byte, passwordLength)
    _, err := rand.Read(password) // Cryptographically secure
    if err != nil {
        return "", fmt.Errorf("failed to generate secure random password: %w", err)
    }
    
    for i := range password {
        password[i] = charset[int(password[i])%len(charset)]
    }
    
    return string(password), nil
}
```

### 2. Input Validation

```go
// api/pkg/types/types.go
func (c *ExternalAgentConfig) Validate() error {
    // Path traversal protection
    if strings.Contains(c.WorkspaceDir, "..") {
        return errors.New("workspace directory cannot contain '..' path components")
    }
    
    // Environment variable whitelisting
    allowedEnvVars := map[string]bool{
        "NODE_ENV": true, "DEBUG": true, "RUST_LOG": true,
        // ... whitelist of safe environment variables
    }
    
    for _, envVar := range c.EnvVars {
        parts := strings.SplitN(envVar, "=", 2)
        key := strings.ToUpper(parts[0])
        if !allowedEnvVars[key] {
            return fmt.Errorf("environment variable not allowed: %s", key)
        }
    }
    
    return nil
}
```

### 3. Access Control

```go
// RDP proxy only allows session owner access
if sessionData.Owner != user.ID {
    http.Error(w, "access denied", http.StatusForbidden)
    return
}
```

## Deployment

### Docker Compose Configuration

```yaml
# docker-compose.dev.yaml
services:
    # Scalable Zed runner pool
    zed-runner:
        build:
            context: .
            dockerfile: Dockerfile.zed-agent
        environment:
            - API_HOST=http://api:8080
            - API_TOKEN=${ZED_AGENT_RUNNER_TOKEN}
            - CONCURRENCY=1      # One session per container
            - MAX_TASKS=1        # Exit after one task
            - WORKSPACE_DIR=/tmp/workspace
            - DISPLAY=:1         # Fixed since container is isolated
            - RDP_PORT=5900      # Fixed since container is isolated
        # No port mapping - RDP proxied over WebSocket
        # No volumes - ephemeral filesystem for clean sessions
        restart: always
        deploy:
            replicas: 3  # Default 3 runners
```

### Scaling

```bash
# Scale to 5 concurrent Zed runners
docker-compose up --scale zed-runner=5

# Scale to 10 concurrent Zed runners
docker-compose up --scale zed-runner=10
```

### Container Lifecycle

```
1. Container starts → Connects to API via WebSocket
2. API dispatches task → Runner receives Zed agent request
3. Runner starts Zed → User works via RDP proxy
4. Task completes → Runner exits gracefully
5. Container restarts → Fresh environment for next session
```

## Usage Examples

### Creating a Session with External Agent

```typescript
// Frontend session creation
const session = await NewInference({
    type: 'text',
    message: 'Create a React todo app',
    agentType: 'zed_external',
    externalAgentConfig: {
        projectPath: 'my-todo-app',
        envVars: ['NODE_ENV=development'],
    },
});
```

### Accessing Remote Desktop

```typescript
// Frontend RDP viewer
<RDPViewer
  sessionId={sessionId}
  onClose={() => setViewerSessionId(null)}
  autoConnect={true}
/>
```

### API Request Flow

```
1. POST /api/v1/sessions/chat
   {
     "agent_type": "zed_external",
     "external_agent_config": { ... },
     "messages": [ ... ]
   }

2. Session handler validates config and creates session

3. LaunchExternalAgent() dispatches to runner pool:
   PubSub.StreamRequest(ZedAgentRunnerStream, ZedAgentQueue, ...)

4. Available runner picks up task via WebSocket

5. Runner starts Zed in container with RDP server

6. Frontend connects: GET /api/v1/external-agents/{sessionID}/rdp/proxy
   → WebSocket proxy over existing runner connection
```

## Security Features

### ✅ Implemented

- **Secure Random Passwords**: 24-character cryptographically secure passwords per session
- **Input Validation**: Path traversal protection, environment variable whitelisting
- **Access Control**: Only session owner can access RDP
- **Container Isolation**: Each session runs in isolated container
- **No Direct Network Access**: All communication via API WebSocket proxy

### ❌ Not Yet Implemented

- **Session Timeouts**: Automatic cleanup of idle sessions
- **Resource Limits**: CPU/memory limits per container
- **Audit Logging**: Security event logging for compliance
- **Network Policies**: Container network isolation

## Architecture Benefits

### Container Isolation
- **Clean Sessions**: Each session starts with fresh container
- **No Data Leakage**: Previous session data automatically cleaned up
- **Resource Isolation**: Each container has isolated filesystem, processes, network

### Scalability
- **Horizontal Scaling**: Add more runners with `--scale zed-runner=N`
- **Load Distribution**: Pub/sub automatically distributes tasks to available runners
- **Graceful Degradation**: Failed containers restart automatically

### Security
- **No Persistent State**: Container filesystem is ephemeral
- **WebSocket Proxy**: No direct RDP network access from frontend
- **User Isolation**: Each session isolated in separate container

## Development vs Production

### Development (Current)
- 3 runner containers by default
- Simplified authentication
- Debug logging enabled

### Production Recommendations
- Scale runners based on load (`--scale zed-runner=20`)
- Add resource limits (`deploy.resources.limits`)
- Enable audit logging
- Add health checks and monitoring
- Implement session timeouts

## Future Enhancements

### Near Term
- **Session Snapshots**: Save/restore workspace state
- **Resource Monitoring**: Track CPU/memory usage per session
- **Auto-scaling**: Dynamic runner scaling based on queue length

### Long Term
- **Multi-host Deployment**: Distribute runners across nodes
- **GPU Support**: GPU-enabled containers for ML workloads
- **Custom Images**: User-defined container images
- **Session Sharing**: Collaborative editing sessions

## Technical Notes

### Package Rename
- Renamed `api/pkg/gptscript` → `api/pkg/zedagent`
- Completely replaces GPTScript functionality
- Single WebSocket server for Zed agents only

### Container Pattern
- No unique display numbers needed (container isolation)
- No port mapping needed (WebSocket proxy)
- No persistent volumes (ephemeral sessions)

### Error Handling
- Failed container startup returns error to user
- Session cleanup on external agent launch failure
- Proper error propagation throughout stack

## Migration from GPTScript

This implementation completely replaces the GPTScript runner system:

| GPTScript Runner | Zed Agent Runner |
|------------------|------------------|
| `/ws/gptscript-runner` | `/ws/zed-runner` |
| `pubsub.ScriptRunnerStream` | `pubsub.ZedAgentRunnerStream` |
| `types.GptScript` | `types.ZedAgent` |
| Script execution | Zed editor + RDP |
| Text-based output | Visual development environment |

The new system maintains the same scaling and reliability characteristics while providing a rich development environment.