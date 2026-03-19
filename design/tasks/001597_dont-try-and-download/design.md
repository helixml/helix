# Design: Embed Sandbox State in the Task List Response

## Root Cause

`useSandboxState` in `ExternalAgentDesktopViewer.tsx` calls `GET /api/v1/sessions/{id}` every 3 seconds per task card. It only uses 4 fields from the response:

- `config.external_agent_status` — "stopped" / "running" / "starting"
- `config.desired_state` — "running" / "stopped"
- `config.container_name` — existence check only
- `config.status_message` — transient message e.g. "Unpacking build cache"

## Approach: Inline in the Task List

The `listTasks` handler (`spec_driven_task_handlers.go:226`) already batch-fetches all sessions via `GetSessionsByIDs` to populate `session_updated_at`. While those sessions are in memory, derive the sandbox state from their config and add it to the task struct. No extra DB queries needed.

### 1. Add fields to `SpecTask` (backend type)

In `api/pkg/types/simple_spec_task.go`, add computed (not stored) fields:

```go
// Sandbox state — populated from session config in listTasks, not stored in DB
SandboxState         string `json:"sandbox_state,omitempty" gorm:"-"`          // "absent", "running", "starting"
SandboxStatusMessage string `json:"sandbox_status_message,omitempty" gorm:"-"` // transient status
```

### 2. Populate in `listTasks`

In the existing session loop in `spec_driven_task_handlers.go`, after setting `task.SessionUpdatedAt`, also derive sandbox state using the same logic currently in `useSandboxState`:

```go
cfg := session.Config
status := cfg.ExternalAgentStatus  // "stopped", "running", "starting"
desiredState := cfg.DesiredState   // "running", "stopped"
hasContainer := cfg.ContainerName != ""

switch {
case status == "stopped" || desiredState == "stopped":
    task.SandboxState = "absent"
case status == "running" || (hasContainer && desiredState == "running"):
    task.SandboxState = "running"
case status == "starting" || (!hasContainer && desiredState == "running"):
    task.SandboxState = "starting"
default:
    task.SandboxState = "absent"
}
task.SandboxStatusMessage = cfg.StatusMessage
```

### 3. Remove `useSandboxState` polling in frontend

`ExternalAgentDesktopViewer.tsx` currently polls independently. Since sandbox state is now in the task object (which the Kanban already refreshes), change `useSandboxState` to accept the state as a prop instead of fetching it. The hook's polling interval (`setInterval` at line 85) is removed entirely.

Pass `task.sandbox_state` and `task.sandbox_status_message` from the task card down into the viewer.

Run `./stack update_openapi` after changing the Go type to regenerate the frontend API client.

## Key Files

| File | Change |
|------|--------|
| `api/pkg/types/simple_spec_task.go` | Add `SandboxState`, `SandboxStatusMessage` fields (gorm:"-") |
| `api/pkg/server/spec_driven_task_handlers.go:243` | Populate sandbox state from session config in existing session loop |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` | Remove polling; accept sandbox state as props |
| `frontend/src/components/tasks/TaskCard.tsx` | Pass `task.sandbox_state` / `task.sandbox_status_message` to viewer |
| `frontend/src/api/api.ts` | Regenerated via `./stack update_openapi` |

### 4. Skip screenshot polling when sandbox is absent

`ScreenshotViewer.tsx` polls `GET /api/v1/external-agents/{sessionId}/screenshot` every 1.7 seconds unconditionally. Add an `enabled` prop (default `true`) that gates the auto-refresh loop. When `sandbox_state` is `"absent"`, pass `enabled={false}` — the component renders the stopped/idle placeholder UI without making any network requests.

```tsx
// In ScreenshotViewer.tsx — gate the polling
if (!autoRefresh || !enabled || streamingMode !== 'screenshot' || sessionUnavailable) return;
```

`ExternalAgentDesktopViewer` already controls whether to render `ScreenshotViewer` vs. the paused-desktop UI. The `sandbox_state` prop it now receives (step 3) is used to set `enabled` accordingly.

## Key Files

| File | Change |
|------|--------|
| `api/pkg/types/simple_spec_task.go` | Add `SandboxState`, `SandboxStatusMessage` fields (gorm:"-") |
| `api/pkg/server/spec_driven_task_handlers.go:243` | Populate sandbox state from session config in existing session loop |
| `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` | Remove polling; accept sandbox state as props; pass `enabled` to ScreenshotViewer |
| `frontend/src/components/external-agent/ScreenshotViewer.tsx:184` | Add `enabled` prop; gate auto-refresh loop on it |
| `frontend/src/components/tasks/TaskCard.tsx` | Pass `task.sandbox_state` / `task.sandbox_status_message` to viewer |
| `frontend/src/api/api.ts` | Regenerated via `./stack update_openapi` |

## Result

Zero calls to `GET /api/v1/sessions/{id}` from the Kanban view. Zero screenshot fetches for stopped desktops. Running desktops still get screenshots at 1.7s cadence. Sandbox state updates at the same pace as the task list refresh.
