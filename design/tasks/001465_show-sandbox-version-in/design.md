# Design: Show Sandbox Version in Admin UI

## Overview

Add Helix version reporting to sandbox heartbeats, display versions in the admin sandbox UI, and alert admins when sandbox versions don't match the control plane.

## Architecture

### Current State
- `sandbox-heartbeat` daemon (Go) sends heartbeats every 30s to `/api/v1/sandboxes/{id}/heartbeat`
- Heartbeat includes: `desktop_versions`, `disk_usage`, `gpu_vendor`, `privileged_mode_enabled`
- Sandbox version is **not** currently reported
- Control plane version available via `data.GetHelixVersion()` (set at build time)

### Proposed Changes

#### 1. Backend: Add `helix_version` to heartbeat

**File: `api/cmd/sandbox-heartbeat/main.go`**
- Add `HelixVersion string` field to `HeartbeatRequest` struct
- Populate with build-time version (import `data.GetHelixVersion()` from `api/pkg/data`)

**File: `api/pkg/types/types.go`**
- Add `HelixVersion string` to `SandboxHeartbeatRequest`
- Add `HelixVersion string` to `SandboxInstance` (stored in DB)

**File: `api/pkg/store/store_sandbox.go`**
- Update `UpdateSandboxHeartbeat` to persist `helix_version`

#### 2. Frontend: Display version in sandbox dropdown

**File: `frontend/src/pages/Dashboard.tsx`**
- Update sandbox `<MenuItem>` to show version: `{hostname} (v{version}) - {sessions}`
- Truncate git hashes to 7 chars for readability

#### 3. Frontend: Global version mismatch alert

**File: `frontend/src/pages/Dashboard.tsx`**
- Compare each sandbox's `helix_version` against `account.serverConfig.version`
- Show `<Alert severity="warning">` at top of agent_sandboxes tab when mismatch detected
- List mismatched sandboxes by hostname

## Data Flow

```
sandbox-heartbeat daemon
    │
    ├── Calls data.GetHelixVersion() at startup
    │
    └── POST /api/v1/sandboxes/{id}/heartbeat
            │
            └── { helix_version: "abc1234", desktop_versions: {...}, ... }
                    │
                    └── store.UpdateSandboxHeartbeat()
                            │
                            └── UPDATE sandbox_instances SET helix_version = ?
```

## Key Decisions

1. **Version source**: Use `data.GetHelixVersion()` which returns git commit hash (or ldflags-injected version). Same as control plane — consistent comparison.

2. **Alert location**: Show alert in agent_sandboxes tab only (not global banner). Admins managing sandboxes will see it; regular users don't need to know.

3. **No blocking**: Version mismatch is warning only. Sandboxes still function even with version mismatch.

4. **DB migration**: Add `helix_version VARCHAR(255)` column to `sandbox_instances` table. GORM AutoMigrate handles this.

## API Changes

### `SandboxHeartbeatRequest` (addition)
```go
type SandboxHeartbeatRequest struct {
    HelixVersion    string            `json:"helix_version,omitempty"`  // NEW
    DesktopVersions map[string]string `json:"desktop_versions,omitempty"`
    // ... existing fields
}
```

### `SandboxInstance` (addition)
```go
type SandboxInstance struct {
    HelixVersion string `json:"helix_version,omitempty" gorm:"type:varchar(255)"`  // NEW
    // ... existing fields
}
```

### Frontend Types (auto-generated)
Run `./stack update_openapi` to regenerate `TypesSandboxInstance` with new field.

## Testing

1. Build sandbox image with updated heartbeat daemon
2. Verify version appears in sandbox list API response
3. Test with mismatched versions (manually edit DB or use old sandbox)
4. Verify alert appears/disappears correctly