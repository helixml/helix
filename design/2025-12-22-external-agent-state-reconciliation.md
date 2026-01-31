# External Agent State Reconciliation System

**Date:** 2025-12-22
**Status:** Draft
**Author:** Claude

## Problem Statement

When Wolf crashes, restarts, or a sandbox container dies unexpectedly, external agent sessions that were meant to be running are left in an inconsistent state:

1. **Session thinks it's running** but the container doesn't exist
2. **No automatic recovery** - users must manually restart sessions
3. **No continuation prompts** - even after restart, agents that were actively working don't resume their work
4. **Frontend shows stale state** - UI shows "connected" but container is gone

Additionally, we need to be careful not to prompt agents that had already finished their work before the crash.

## Goals

1. **Automatic restart** of sessions that should be running after Wolf crash/restart
2. **Continue prompts** sent to agents once Zed WebSocket reconnects to control plane
3. **No unnecessary prompts** to agents that had already finished work
4. **State lives in backend** - not reliant on frontend for agent active/finished detection

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     EXTERNAL AGENT RECONCILIATION FLOW                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. DESIRED STATE TRACKING (new fields on session/activity)                 │
│     ┌──────────────────────────────────────────────────────────────────┐   │
│     │ Session.Metadata.DesiredState: "running" | "stopped"             │   │
│     │ ExternalAgentActivity.AgentWorkState: "idle" | "working" | "done"│   │
│     └──────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  2. RECONCILIATION LOOP (background goroutine, 30s interval)                │
│     ┌──────────────────────────────────────────────────────────────────┐   │
│     │ For each session where DesiredState == "running":                │   │
│     │   - Check if Wolf has container running (via Wolf API)          │   │
│     │   - If not: call externalAgentExecutor.StartDesktop()           │   │
│     └──────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  3. WEBSOCKET RECONNECT HANDLER (existing handleExternalAgentSync)         │
│     ┌──────────────────────────────────────────────────────────────────┐   │
│     │ On WebSocket connect:                                            │   │
│     │   - Check AgentWorkState from database                           │   │
│     │   - If "working": send continue prompt to agent                  │   │
│     │   - If "idle" or "done": no prompt (just restore mappings)      │   │
│     └──────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Detailed Design

### 1. New Types and Fields

#### 1.1 DesiredState (session-level)

Add to `types.SessionMetadata`:

```go
// api/pkg/types/session.go
type SessionMetadata struct {
    // ... existing fields ...

    // DesiredState indicates whether this session's container should be running
    // "running" = container should exist, reconciler will restart if missing
    // "stopped" = container can be terminated, no auto-restart
    DesiredState string `json:"desired_state,omitempty"`
}
```

**When DesiredState is set:**
- `"running"`: Set when `StartDesktop()` is called for spec generation or implementation
- `"stopped"`: Set when user explicitly stops session, or task is archived/completed

#### 1.2 AgentWorkState (activity-level)

Add to `types.ExternalAgentActivity`:

```go
// api/pkg/types/simple_spec_task.go
type AgentWorkState string

const (
    AgentWorkStateIdle    AgentWorkState = "idle"    // Agent connected but not actively working
    AgentWorkStateWorking AgentWorkState = "working" // Agent actively processing a prompt
    AgentWorkStateDone    AgentWorkState = "done"    // Agent finished its assigned task
)

type ExternalAgentActivity struct {
    // ... existing fields ...

    // AgentWorkState tracks whether the agent was actively working
    // Used to decide whether to send a continue prompt after restart
    AgentWorkState AgentWorkState `json:"agent_work_state" gorm:"size:50;default:'idle'"`

    // LastPromptContent stores the last prompt sent to the agent (for continue prompts)
    // Only stored when AgentWorkState is "working"
    LastPromptContent string `json:"last_prompt_content,omitempty" gorm:"type:text"`
}
```

**State Transitions:**

```
                    ┌───────────────────┐
                    │      idle         │ ← Initial state on connect
                    └─────────┬─────────┘
                              │ chat_message sent
                              ▼
                    ┌───────────────────┐
                    │     working       │ ← Prompt in flight
                    └─────────┬─────────┘
                              │ message_completed received
                              ▼
                    ┌───────────────────┐
              ┌────►│      idle         │ ← Ready for next prompt
              │     └─────────┬─────────┘
              │               │ task marked complete/archived
              │               ▼
              │     ┌───────────────────┐
              │     │      done         │ ← Terminal state
              │     └───────────────────┘
              │
              └── (another prompt sent) ──┘
```

### 2. Reconciliation Service

Create new service: `api/pkg/services/external_agent_reconciler.go`

```go
package services

import (
    "context"
    "time"

    "github.com/helixml/helix/api/pkg/external-agent"
    "github.com/helixml/helix/api/pkg/store"
    "github.com/helixml/helix/api/pkg/types"
    "github.com/rs/zerolog/log"
)

const (
    ReconcileInterval = 30 * time.Second
    ContainerStartTimeout = 5 * time.Minute
)

// ExternalAgentReconciler ensures external agent containers match desired state
type ExternalAgentReconciler struct {
    store           store.Store
    executor        external_agent.Executor
    specTaskService *SpecDrivenTaskService
}

func NewExternalAgentReconciler(
    store store.Store,
    executor external_agent.Executor,
    specTaskService *SpecDrivenTaskService,
) *ExternalAgentReconciler {
    return &ExternalAgentReconciler{
        store:           store,
        executor:        executor,
        specTaskService: specTaskService,
    }
}

// Start begins the reconciliation loop
func (r *ExternalAgentReconciler) Start(ctx context.Context) {
    log.Info().Msg("Starting external agent reconciler")

    ticker := time.NewTicker(ReconcileInterval)
    defer ticker.Stop()

    // Run once immediately on startup
    r.reconcile(ctx)

    for {
        select {
        case <-ctx.Done():
            log.Info().Msg("Stopping external agent reconciler")
            return
        case <-ticker.C:
            r.reconcile(ctx)
        }
    }
}

// reconcile performs a single reconciliation pass
func (r *ExternalAgentReconciler) reconcile(ctx context.Context) {
    // Get all sessions with DesiredState = "running"
    sessions, err := r.store.ListSessionsWithDesiredState(ctx, "running")
    if err != nil {
        log.Error().Err(err).Msg("Failed to list sessions with desired state running")
        return
    }

    if len(sessions) == 0 {
        log.Debug().Msg("Reconcile: no sessions need running containers")
        return
    }

    log.Info().Int("count", len(sessions)).Msg("Reconciling external agent sessions")

    for _, session := range sessions {
        if err := r.reconcileSession(ctx, session); err != nil {
            log.Error().
                Err(err).
                Str("session_id", session.ID).
                Msg("Failed to reconcile session")
        }
    }
}

// reconcileSession ensures a single session's container matches desired state
func (r *ExternalAgentReconciler) reconcileSession(ctx context.Context, session *types.Session) error {
    // Check if Wolf has a running container for this session
    hasContainer := r.executor.HasRunningContainer(ctx, session.ID)

    if hasContainer {
        // Container exists, nothing to do
        log.Debug().
            Str("session_id", session.ID).
            Msg("Container already running, nothing to reconcile")
        return nil
    }

    // Container missing but should be running - restart it
    log.Info().
        Str("session_id", session.ID).
        Str("spec_task_id", session.Metadata.SpecTaskID).
        Msg("Container missing, restarting session")

    // Get the spec task to determine restart method
    if session.Metadata.SpecTaskID == "" {
        log.Warn().
            Str("session_id", session.ID).
            Msg("Session has no SpecTaskID, cannot restart")
        return nil
    }

    task, err := r.store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
    if err != nil {
        return fmt.Errorf("failed to get spec task: %w", err)
    }

    // Use the existing resume session logic
    // This calls the Wolf executor to start a new container
    err = r.specTaskService.ResumeSession(ctx, task, session)
    if err != nil {
        return fmt.Errorf("failed to resume session: %w", err)
    }

    log.Info().
        Str("session_id", session.ID).
        Str("spec_task_id", task.ID).
        Msg("Successfully restarted session container")

    return nil
}
```

### 3. WebSocket Reconnect Continue Handler

Modify `api/pkg/server/websocket_external_agent_sync.go` in `handleExternalAgentSync`:

```go
// handleExternalAgentSync handles WebSocket connections from external agents (Zed instances)
func (apiServer *HelixAPIServer) handleExternalAgentSync(res http.ResponseWriter, req *http.Request) {
    // ... existing connection setup code ...

    // Register connection
    apiServer.externalAgentWSManager.registerConnection(agentID, wsConn)
    defer apiServer.externalAgentWSManager.unregisterConnection(agentID)

    // NEW: Check if this agent needs a continue prompt
    if helixSessionID != "" {
        apiServer.sendContinuePromptIfNeeded(ctx, helixSessionID, wsConn)
    }

    // ... rest of existing code ...
}

// sendContinuePromptIfNeeded checks agent work state and sends continue prompt if agent was working
func (apiServer *HelixAPIServer) sendContinuePromptIfNeeded(ctx context.Context, sessionID string, wsConn *ExternalAgentWSConnection) {
    // Get activity record to check work state
    activity, err := apiServer.Store.GetExternalAgentActivity(ctx, sessionID)
    if err != nil || activity == nil {
        log.Debug().
            Str("session_id", sessionID).
            Msg("No activity record found, skipping continue prompt")
        return
    }

    // Only send continue prompt if agent was actively working
    if activity.AgentWorkState != types.AgentWorkStateWorking {
        log.Debug().
            Str("session_id", sessionID).
            Str("work_state", string(activity.AgentWorkState)).
            Msg("Agent was not working, no continue prompt needed")
        return
    }

    log.Info().
        Str("session_id", sessionID).
        Str("spec_task_id", activity.SpecTaskID).
        Msg("Agent was working before disconnect, sending continue prompt")

    // Get session for thread ID
    session, err := apiServer.Store.GetSession(ctx, sessionID)
    if err != nil {
        log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for continue prompt")
        return
    }

    // Build continue prompt
    continueMessage := `The sandbox was restarted. Please continue working on your current task.

If you were in the middle of something, please resume from where you left off.
If you need to verify the current state, check the git status and any running processes.`

    // Determine agent name
    agentName := apiServer.getAgentNameForSession(ctx, session)

    command := types.ExternalAgentCommand{
        Type: "chat_message",
        Data: map[string]interface{}{
            "message":       continueMessage,
            "request_id":    system.GenerateRequestID(),
            "acp_thread_id": session.Metadata.ZedThreadID,
            "agent_name":    agentName,
            "is_continue":   true, // Flag so agent knows this is a recovery prompt
        },
    }

    // Send via channel
    select {
    case wsConn.SendChan <- command:
        log.Info().
            Str("session_id", sessionID).
            Msg("Sent continue prompt to agent after reconnect")
    default:
        log.Warn().
            Str("session_id", sessionID).
            Msg("Failed to send continue prompt - channel full")
    }
}
```

### 4. Work State Tracking Updates

Modify existing handlers to track work state:

#### 4.1 When sending prompts (set to "working")

In `NotifyExternalAgentOfNewInteraction`:

```go
func (apiServer *HelixAPIServer) NotifyExternalAgentOfNewInteraction(sessionID string, interaction *types.Interaction) error {
    // ... existing code ...

    // NEW: Update work state to "working" and store prompt
    activity, _ := apiServer.Store.GetExternalAgentActivity(context.Background(), sessionID)
    if activity != nil {
        activity.AgentWorkState = types.AgentWorkStateWorking
        activity.LastPromptContent = interaction.PromptMessage
        activity.LastInteraction = time.Now()
        apiServer.Store.UpsertExternalAgentActivity(context.Background(), activity)
    }

    // ... send command ...
}
```

#### 4.2 When message completes (set to "idle")

In `handleMessageCompleted`:

```go
func (apiServer *HelixAPIServer) handleMessageCompleted(sessionID string, syncMsg *types.SyncMessage) error {
    // ... existing code ...

    // NEW: Update work state to "idle"
    activity, err := apiServer.Store.GetExternalAgentActivity(context.Background(), sessionID)
    if err == nil && activity != nil {
        activity.AgentWorkState = types.AgentWorkStateIdle
        activity.LastPromptContent = "" // Clear - work is complete
        activity.LastInteraction = time.Now()
        apiServer.Store.UpsertExternalAgentActivity(context.Background(), activity)
    }

    // ... rest of existing code ...
}
```

#### 4.3 When task completes (set to "done")

In spec task handlers when status changes to `done` or `archived`:

```go
// In UpdateSpecTask handler or service
func (s *SpecDrivenTaskService) MarkTaskDone(ctx context.Context, task *types.SpecTask) error {
    // ... update task status ...

    // Mark agent work state as done (prevents continue prompts)
    if task.PlanningSessionID != "" {
        activity, err := s.store.GetExternalAgentActivity(ctx, task.PlanningSessionID)
        if err == nil && activity != nil {
            activity.AgentWorkState = types.AgentWorkStateDone
            s.store.UpsertExternalAgentActivity(ctx, activity)
        }

        // Also set desired state to stopped
        session, err := s.store.GetSession(ctx, task.PlanningSessionID)
        if err == nil && session != nil {
            session.Metadata.DesiredState = "stopped"
            s.store.UpdateSession(ctx, *session)
        }
    }

    return nil
}
```

### 5. Store Interface Extensions

Add to `api/pkg/store/store.go`:

```go
// Store interface additions
type Store interface {
    // ... existing methods ...

    // ListSessionsWithDesiredState returns sessions where metadata.desired_state matches
    ListSessionsWithDesiredState(ctx context.Context, desiredState string) ([]*types.Session, error)
}
```

Implementation in `api/pkg/store/store_sessions.go`:

```go
// ListSessionsWithDesiredState returns sessions where desired_state matches
func (s *PostgresStore) ListSessionsWithDesiredState(ctx context.Context, desiredState string) ([]*types.Session, error) {
    var sessions []*types.Session

    // PostgreSQL JSONB query for metadata.desired_state
    err := s.gdb.WithContext(ctx).
        Where("metadata->>'desired_state' = ?", desiredState).
        Find(&sessions).Error

    return sessions, err
}
```

### 6. Wolf Executor Extensions

Add to `api/pkg/external-agent/wolf_client_interface.go`:

```go
type Executor interface {
    // ... existing methods ...

    // HasRunningContainer checks if Wolf has a running container for this session
    HasRunningContainer(ctx context.Context, sessionID string) bool
}
```

Implementation checks Wolf's `/api/v1/apps` endpoint for matching wolf_lobby_id.

### 7. Integration Points

#### 7.1 Start Reconciler on API Startup

In `api/cmd/helix/root.go` or server initialization:

```go
// After creating specDrivenTaskService
reconciler := services.NewExternalAgentReconciler(
    store,
    externalAgentExecutor,
    specDrivenTaskService,
)
go reconciler.Start(ctx)
```

#### 7.2 Set DesiredState on Session Creation

In `StartSpecGeneration` and `StartJustDoItMode`:

```go
session := &types.Session{
    // ... existing fields ...
    Metadata: types.SessionMetadata{
        // ... existing fields ...
        DesiredState: "running", // NEW: Mark as should-be-running
    },
}
```

## Migration Plan

1. **Add new fields** via GORM AutoMigrate (no SQL migration needed)
2. **Backfill existing sessions** - Default to `DesiredState: "stopped"` for existing sessions
3. **Backfill activity records** - Default to `AgentWorkState: "idle"` for existing records
4. **Enable reconciler** - Start with longer interval (5 min) then reduce to 30s

## Testing Strategy

1. **Unit tests** for state transitions
2. **Integration test**: Kill Wolf container, verify sessions restart
3. **Integration test**: Kill sandbox container, verify WebSocket reconnects and sends continue prompt
4. **Integration test**: Archive task, verify no continue prompt on reconnect
5. **Load test**: Many sessions reconciling simultaneously

## Observability

Add metrics:
- `helix_reconciler_sessions_restarted_total` - Counter of sessions restarted
- `helix_reconciler_continue_prompts_sent_total` - Counter of continue prompts sent
- `helix_reconciler_reconcile_duration_seconds` - Histogram of reconcile loop duration

Add logs:
- INFO when restarting a session
- INFO when sending continue prompt
- WARN when restart fails
- DEBUG for healthy sessions (no action needed)

## Security Considerations

1. **Rate limiting** - Don't restart sessions in a tight loop if they keep failing
2. **User context** - Ensure restarted sessions maintain original user's permissions
3. **Audit logging** - Log when sessions are auto-restarted

## Open Questions

1. **Retry backoff**: How long to wait before retrying a failed restart?
   - Proposal: Exponential backoff with max 5 minutes

2. **Max restart attempts**: After how many failures should we give up?
   - Proposal: 5 attempts, then mark session as failed and alert user

3. **Continue prompt customization**: Should the continue prompt include task context?
   - Proposal: Include task name and last known state for better context

## Alternative Approaches Considered

### A. Event-Driven via Wolf WebSocket
Instead of polling, subscribe to Wolf container lifecycle events. Rejected because Wolf doesn't currently expose lifecycle webhooks.

### B. Kubernetes-style Controller Pattern
Full declarative reconciliation with status subresource. Rejected as over-engineering for current scale.

### C. Frontend-Initiated Recovery
Let frontend detect disconnect and trigger restart. Rejected because user must have browser open, and state detection is unreliable in frontend.

## 8. Frontend Changes - Remove Duplicate Attention Logic

### Current Frontend Logic (to be removed)

The frontend currently has duplicate logic for detecting agent activity in `frontend/src/components/tasks/TaskCard.tsx`:

```typescript
// CURRENT: Heuristic-based detection using timestamps (REMOVE THIS)
const isAgentActive = (sessionUpdatedAt?: string): boolean => {
  if (!sessionUpdatedAt) return false
  const updatedTime = new Date(sessionUpdatedAt).getTime()
  const now = Date.now()
  const diffSeconds = (now - updatedTime) / 1000
  return diffSeconds < 10 // Active if updated within last 10 seconds
}

const useAgentActivityCheck = (sessionUpdatedAt?: string, enabled: boolean = true) => {
  const [tick, setTick] = useState(0)
  const [lastSeenTimestamp, setLastSeenTimestamp] = useState<string | null>(null)

  // Polls every 3 seconds to re-check
  useEffect(() => {
    const interval = setInterval(() => setTick(t => t + 1), 3000)
    return () => clearInterval(interval)
  }, [enabled, sessionUpdatedAt])

  const isActive = isAgentActive(sessionUpdatedAt, tick)
  const needsAttention = !isActive && sessionUpdatedAt !== lastSeenTimestamp
  // ...
}
```

**Problems with current approach:**
1. Timestamp-based heuristic is unreliable (what if agent is thinking for 15 seconds?)
2. Polling every 3 seconds wastes resources
3. Local React state for `lastSeenTimestamp` doesn't persist across page refreshes
4. Duplicate logic that doesn't match backend truth

### New Frontend Logic (using backend state)

#### 8.1 Extend API Response

Add `agent_work_state` to the SpecTask API response:

```go
// api/pkg/types/simple_spec_task.go
type SpecTask struct {
    // ... existing fields ...

    // AgentWorkState is populated from ExternalAgentActivity when task is fetched
    // Read-only computed field, not stored on SpecTask itself
    AgentWorkState AgentWorkState `json:"agent_work_state,omitempty" gorm:"-"`
}
```

Populate in the API handler:

```go
// api/pkg/server/spec_task_handlers.go
func (s *HelixAPIServer) populateAgentWorkState(ctx context.Context, task *types.SpecTask) {
    if task.PlanningSessionID == "" {
        return
    }
    activity, err := s.Store.GetExternalAgentActivity(ctx, task.PlanningSessionID)
    if err == nil && activity != nil {
        task.AgentWorkState = activity.AgentWorkState
    }
}
```

#### 8.2 Simplify Frontend Hook

Replace the complex hook with a simple state reader:

```typescript
// frontend/src/components/tasks/TaskCard.tsx

// NEW: Simple hook that reads backend state
const useAgentActivityState = (task: SpecTaskWithExtras) => {
  // Backend provides the authoritative state
  const workState = task.agent_work_state || 'idle'

  // Derive display properties from backend state
  const isActive = workState === 'working'
  const isDone = workState === 'done'

  // Needs attention = was working, now idle (but not done)
  // The backend sets this when message_completed fires
  const needsAttention = workState === 'idle' &&
    task.planning_session_id &&
    (task.phase === 'planning' || task.phase === 'implementation')

  return { isActive, isDone, needsAttention }
}
```

#### 8.3 Update TaskCard Component

```typescript
// Replace current usage:
// const { isActive, needsAttention, markAsSeen } = useAgentActivityCheck(...)

// With new simple hook:
const { isActive, isDone, needsAttention } = useAgentActivityState(task)

// Remove markAsSeen - backend handles attention state
// Clicking the card sends an API call to acknowledge (optional enhancement)
```

#### 8.4 Update SpecTask Interface

```typescript
// frontend/src/components/tasks/TaskCard.tsx
interface SpecTaskWithExtras {
  // ... existing fields ...

  // NEW: Backend-provided work state
  agent_work_state?: 'idle' | 'working' | 'done'
}
```

#### 8.5 Optional: Acknowledge Attention Endpoint

If we want to persist "user saw this idle state", add an endpoint:

```go
// POST /api/v1/spec-tasks/{taskId}/acknowledge
func (s *HelixAPIServer) acknowledgeTaskAttention(w http.ResponseWriter, r *http.Request) {
    taskID := mux.Vars(r)["taskId"]
    task, _ := s.Store.GetSpecTask(ctx, taskID)

    activity, _ := s.Store.GetExternalAgentActivity(ctx, task.PlanningSessionID)
    if activity != nil {
        activity.AttentionAcknowledgedAt = time.Now()
        s.Store.UpsertExternalAgentActivity(ctx, activity)
    }
}
```

Then frontend checks: `needsAttention = workState === 'idle' && !activity.AttentionAcknowledgedAt`

### Summary of Frontend Changes

| File | Change |
|------|--------|
| `TaskCard.tsx` | Remove `useAgentActivityCheck`, `isAgentActive` functions |
| `TaskCard.tsx` | Add simple `useAgentActivityState` hook |
| `TaskCard.tsx` | Update `SpecTaskWithExtras` interface |
| `specTaskService.ts` | No changes (work state comes with task fetch) |

**Benefits:**
- Single source of truth (backend)
- No polling (state refreshes with task data)
- Survives page refresh (persisted in DB)
- Accurate state (tracks actual work, not timestamps)

## References

- `api/pkg/store/wolf_health_monitor.go` - Pattern for background reconciliation loops
- `api/pkg/server/websocket_external_agent_sync.go` - WebSocket connection handling
- `api/pkg/services/spec_driven_task_service.go` - Session creation for spec tasks
- `frontend/src/components/tasks/TaskCard.tsx` - Current attention indicator logic (to be replaced)
