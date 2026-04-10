# Design: "Keep Alive" Toggle

## Architecture

This is a simple boolean flag on SpecTask that the idle checker respects. Three touch points: database model, idle checker query, frontend toggle.

## Backend Changes

### 1. Add `KeepAlive` field to SpecTask model

**File:** `api/pkg/types/simple_spec_task.go`

Add to the SpecTask struct (near other boolean flags like `JustDoItMode`, `PublicDesignDocs`):

```go
KeepAlive bool `json:"keep_alive" gorm:"default:false"` // Prevent auto-idle-shutdown
```

GORM AutoMigrate will add the column automatically.

### 2. Add `KeepAlive` to SpecTaskUpdateRequest

**File:** `api/pkg/types/simple_spec_task.go`

```go
KeepAlive *bool `json:"keep_alive,omitempty"` // Pointer to allow explicit false
```

### 3. Handle in updateSpecTask handler

**File:** `api/pkg/server/spec_driven_task_handlers.go` (around line 919, after PublicDesignDocs handling)

```go
if updateReq.KeepAlive != nil {
    task.KeepAlive = *updateReq.KeepAlive
}
```

### 4. Modify idle checker SQL query to skip keep-alive tasks

**File:** `api/pkg/store/store_sessions.go`, `ListIdleDesktops()` (line 349)

The current query finds idle desktops by joining `sessions` with `interactions`. We need to exclude sessions whose parent spectask has `keep_alive = true`.

Add a LEFT JOIN + WHERE filter against `spec_tasks`:

```sql
-- In the CTE WHERE clause, add:
AND NOT EXISTS (
    SELECT 1 FROM spec_tasks st
    WHERE st.planning_session_id = s.id
      AND st.keep_alive = true
      AND st.deleted_at IS NULL
)
```

This approach:
- Uses the existing `planning_session_id` link between SpecTask and Session
- Doesn't require changes to the idle checker Go code — the query simply returns fewer results
- Has no performance impact (spec_tasks table is small, planning_session_id is indexed)

### 5. Clear keep_alive on task reset to backlog

**File:** `api/pkg/server/spec_driven_task_handlers.go`

In the backlog reset block (line 855), add: `task.KeepAlive = false`

## Frontend Changes

### Toggle button in header toolbar

**File:** `frontend/src/components/tasks/SpecTaskDetailContent.tsx`

Add a toggle button in the top-right action buttons area (after the Stop button, before Upload), visible only when desktop is running:

```tsx
{isDesktopRunning && (
  <Tooltip title={task.keep_alive ? "Keep Alive is ON — container won't auto-sleep" : "Keep Alive is OFF — container will auto-sleep after idle timeout"}>
    <IconButton
      size="small"
      onClick={() => updateSpecTask.mutate({ keep_alive: !task.keep_alive })}
      sx={{ color: task.keep_alive ? 'success.main' : 'text.secondary' }}
    >
      {task.keep_alive ? <LockIcon /> : <LockOpenIcon />}
    </IconButton>
  </Tooltip>
)}
```

**Icon choice:** MUI `Lock` (filled, green when active) / `LockOpen` (grey when inactive). Even though we're not using the word "lock" in the label, the lock icon visually communicates "pinned/protected" which pairs well with the "Keep Alive" tooltip.

**Alternative icon:** Could use `PowerSettingsNew` or a custom "heartbeat" icon, but Lock/LockOpen is already in MUI and immediately recognizable.

### API call

Uses the existing `updateSpecTask` mutation from `specTaskService.ts` — the same mutation used for other SpecTask field updates. No new API endpoint needed. The generated API client will include `keep_alive` after regenerating from the updated swagger annotations.

## Codebase Patterns Found

- **Boolean toggle pattern:** Follows exactly the same pattern as `PublicDesignDocs` — stored as `bool` with `gorm:"default:false"`, updated via pointer field `*bool` in `SpecTaskUpdateRequest`, handled in `updateSpecTask` handler
- **Idle checker:** `api/pkg/external-agent/idle_checker.go` calls `store.ListIdleDesktops()` — all filtering happens in SQL, so the Go code doesn't need changes
- **Header toolbar:** Action buttons are in a `<Box sx={{ display: "flex", gap: 0.5 }}>` at line 2033 of `SpecTaskDetailContent.tsx`
- **API client regen:** Run `./stack update_openapi` after adding swagger annotations to regenerate the TypeScript client

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where to store | SpecTask model (not Session) | Setting is per-task, survives session restarts |
| How to filter | SQL NOT EXISTS in ListIdleDesktops | No Go code changes, clean separation |
| UI placement | Header toolbar, near stop button | Visible and accessible, consistent with existing controls |
| Default | Off (false) | Safe default — resources still auto-cleaned |
| Naming | "Keep Alive" | Clear, familiar, not ambiguous like "lock" |
