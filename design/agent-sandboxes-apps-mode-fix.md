# Agent Sandboxes Dashboard - Apps Mode Support Fix

**Date**: 2025-10-13
**Issue**: Dashboard returning "Wolf executor not available" error after apps/lobbies mode split

## Problem

After implementing the WOLF_MODE toggle between apps and lobbies executors, the Agent Sandboxes dashboard failed to load with error:
```
Wolf executor not available
```

## Root Cause

Go interface type mismatch in `api/pkg/server/agent_sandboxes_handlers.go`:

```go
// Handler defined interface with generic return type
type WolfClientProvider interface {
    GetWolfClient() interface{}  // Generic interface{}
}

// But implementations returned concrete type
func (w *WolfExecutor) GetWolfClient() *wolf.Client      // Concrete type
func (w *AppWolfExecutor) GetWolfClient() *wolf.Client   // Concrete type
```

In Go, `*wolf.Client` does NOT satisfy `interface{}` as a return type - method signatures must match exactly. The type assertion `apiServer.externalAgentExecutor.(WolfClientProvider)` failed because neither executor implemented the interface as defined.

## Solution

Changed handler to use concrete type with proper import:

### 1. Added wolf package import
```go
import (
    // ... existing imports
    "github.com/helixml/helix/api/pkg/wolf"
)
```

### 2. Fixed interface definition
```go
type WolfClientProvider interface {
    GetWolfClient() *wolf.Client  // Concrete type
}
```

### 3. Simplified helper functions
Removed unnecessary type assertions from `fetchWolfMemoryData()`, `fetchWolfApps()`, `fetchWolfLobbies()`, and `fetchWolfSessions()` - they now accept `*wolf.Client` directly.

## Additional Improvements

### Explicit wolf_mode field
Added explicit mode indicator to response instead of inferring from data structure:

**Backend** (`agent_sandboxes_handlers.go`):
```go
type AgentSandboxesDebugResponse struct {
    Memory           *WolfSystemMemory      `json:"memory"`
    Apps             []WolfAppInfo          `json:"apps,omitempty"`
    Lobbies          []WolfLobbyInfo        `json:"lobbies,omitempty"`
    Sessions         []WolfSessionInfo      `json:"sessions"`
    MoonlightClients []MoonlightClientInfo  `json:"moonlight_clients"`
    WolfMode         string                 `json:"wolf_mode"`  // NEW: "apps" or "lobbies"
}

// Set in handler
response.WolfMode = wolfMode
```

**Frontend** (`AgentSandboxes.tsx`):
```typescript
// Changed from inferring mode (broken when apps array is empty):
const isAppsMode = apps.length > 0  // ❌ Fails when no apps exist

// To explicit mode from backend:
const isAppsMode = data?.wolf_mode === 'apps'  // ✅ Always correct
```

### Type corrections
Also fixed memory byte fields from `string` to `int64` for proper numeric handling:
```go
type WolfSystemMemory struct {
    ProcessRSSBytes      int64  `json:"process_rss_bytes"`      // Was string
    GStreamerBufferBytes int64  `json:"gstreamer_buffer_bytes"` // Was string
    TotalMemoryBytes     int64  `json:"total_memory_bytes"`     // Was string
}
```

## Result

✅ Dashboard loads successfully in apps mode
✅ Mode detection works even with empty apps array
✅ Cleaner code without unnecessary type assertions
✅ Frontend has explicit mode indicator from backend

## Files Modified

- `api/pkg/server/agent_sandboxes_handlers.go` - Interface fix, wolf_mode field
- `frontend/src/components/admin/AgentSandboxes.tsx` - Use explicit wolf_mode
- API hot reloader picked up changes immediately, no container rebuild needed
