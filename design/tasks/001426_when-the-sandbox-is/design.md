# Design: Session State After Sandbox Restart

## Overview

When a sandbox restarts, running containers are destroyed but the API's database still has stale metadata (`container_name`, `container_id`, etc.). This causes the frontend to show an inconsistent state instead of a clean "Stopped" UI.

## Root Cause Analysis

**Current flow when sandbox disconnects:**
1. `connman.OnDisconnect(key)` is called → starts grace period
2. Grace period expires → connection removed from `deviceDialers`
3. **Problem**: Session DB metadata (`container_name`, `external_agent_status`) is NOT updated
4. Frontend polls session → sees `container_name` set → thinks container exists
5. Calls `GetSession()` on executor → fails → sets `external_agent_status="stopped"`
6. But `container_name` is still set → logic confusion in frontend

**Key insight**: The stale `container_name` in the database causes the `hasContainer` check to be true in the frontend, even though the container no longer exists.

## Solution

### Option A: Clear Metadata on Sandbox Disconnect (Recommended)

When the sandbox disconnects and grace period expires, proactively clear session metadata for all sessions on that sandbox.

**Pros:**
- Clean state immediately visible
- No polling/timeout needed
- Frontend logic remains simple

**Cons:**
- Requires tracking which sessions are on which sandbox (already tracked via `session.SandboxID`)

### Option B: Backend-Only Status Check

Keep current approach but fix the frontend logic to trust `external_agent_status` over `container_name`.

**Pros:**
- Smaller change (frontend only)

**Cons:**
- Doesn't fix root cause (stale data in DB)
- Other consumers of session data may be confused

## Chosen Approach: Option A

Clear session metadata when sandbox disconnects.

## Architecture

```
Sandbox Disconnect Flow:
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   ConnMan    │────▶│   Callback   │────▶│    Store     │
│ OnDisconnect │     │   Handler    │     │ ClearSessions│
└──────────────┘     └──────────────┘     └──────────────┘
                                                  │
                                                  ▼
                                          ┌──────────────┐
                                          │   Sessions   │
                                          │ container_*  │
                                          │   = NULL     │
                                          └──────────────┘
```

## Implementation Details

### 1. Add Callback to ConnectionManager

Add an optional `OnGracePeriodExpired` callback to `connman.ConnectionManager`:

```go
type ConnectionManager struct {
    // ... existing fields
    onGracePeriodExpired func(key string) // Called when grace period expires
}
```

### 2. Clear Session Metadata

When sandbox grace period expires, clear container metadata for affected sessions:

```go
// In hydra_executor.go or a new service
func (h *HydraExecutor) OnSandboxDisconnected(sandboxID string) {
    sessions, _ := h.store.ListSessionsBySandbox(ctx, sandboxID)
    for _, session := range sessions {
        session.Metadata.ContainerName = ""
        session.Metadata.ContainerID = ""
        session.Metadata.ContainerIP = ""
        session.Metadata.ExternalAgentStatus = "stopped"
        // Keep DesiredState unchanged for reconciler
        h.store.UpdateSession(ctx, *session)
    }
    // Also clear in-memory sessions map
    h.clearSessionsBySandbox(sandboxID)
}
```

### 3. Wire Up the Callback

In server initialization, register the callback:

```go
connman.SetOnGracePeriodExpired(func(key string) {
    if strings.HasPrefix(key, "hydra-") {
        sandboxID := strings.TrimPrefix(key, "hydra-")
        executor.OnSandboxDisconnected(sandboxID)
    }
})
```

## Data Flow

1. **Sandbox disconnects** → `connman.OnDisconnect("hydra-sandbox123")`
2. **Grace period (30s)** → waits for reconnection
3. **Grace period expires** → `onGracePeriodExpired("hydra-sandbox123")` called
4. **Clear sessions** → All sessions with `sandbox_id=sandbox123` get container metadata cleared
5. **Frontend polls** → Sees `container_name=""`, `external_agent_status="stopped"` → Shows "Paused" UI
6. **Sandbox reconnects later** → Reconciler sees `desired_state=running`, container missing → Restarts

## Testing

1. Start a session, verify running
2. Restart sandbox (`docker compose restart sandbox-nvidia`)
3. Verify session shows "Paused" in UI (not spinner/error)
4. Click "Resume" → verify session restarts successfully

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Grace period too short (sandbox rebooting) | 30s grace period is sufficient for most restarts |
| Race condition with reconnection | Check if sandbox reconnected before clearing |
| Multiple sandboxes | Use `sandbox_id` field to target correct sessions |

## Implementation Notes

### Files Modified

| File | Changes |
|------|---------|
| `api/pkg/connman/connman.go` | Added `onGracePeriodExpired` callback field, `SetOnGracePeriodExpired()` method, callback invocation in `cleanupExpired()` |
| `api/pkg/connman/connman_test.go` | Added 3 unit tests for callback behavior |
| `api/pkg/store/store.go` | Added `ListSessionsBySandbox()` to Store interface |
| `api/pkg/store/store_sessions.go` | Implemented `ListSessionsBySandbox()` for PostgresStore |
| `api/pkg/store/store_mocks.go` | Added mock for `ListSessionsBySandbox()` |
| `api/pkg/external-agent/hydra_executor.go` | Added `OnSandboxDisconnected()` and `clearSessionsBySandbox()` methods |
| `api/pkg/server/server.go` | Wired up callback in `NewServer()` initialization |

### Key Patterns Used

- **Callback outside lock**: The `cleanupExpired()` method releases the lock before calling the callback to avoid deadlocks
- **Background context**: `OnSandboxDisconnected()` creates its own context since the callback may be invoked from a goroutine
- **Graceful cleanup**: `SandboxID` is cleared along with container metadata so sessions aren't re-associated with the wrong sandbox

### Gotchas

- The callback key format is `hydra-{sandboxID}` - must strip prefix to get actual sandbox ID
- Must clear both database metadata AND in-memory sessions map
- `DesiredState` is intentionally NOT cleared so the reconciler can restart sessions when sandbox returns