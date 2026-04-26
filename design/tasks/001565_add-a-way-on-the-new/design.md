# Design: Auto-Start Option on New Spec Task Form

## UX Decision

**Chosen approach: Checkbox (same pattern as "Just Do It" mode)**

Options considered:
- Split-button dropdown on "Create Task" — adds interaction complexity and hides the option until clicked
- Second button ("Create & Start") — creates two code paths and clutters the action area
- **Checkbox** — consistent with the existing "Just Do It" checkbox, always visible, intent is explicit

The checkbox is placed below "Just Do It" in the form. Both checkboxes relate to task-start behaviour, so grouping them is logical. Label: **"Start immediately"** with sub-text: *"Begin spec generation as soon as the task is created, regardless of the project's auto-start setting."*

The existing "Just Do It" checkbox uses `color="warning"` (orange) to signal "skip steps". The new checkbox can use `color="primary"` (blue) to signal "start now, but normally" — visually distinct.

## Backend Changes

### `api/pkg/types/simple_spec_task.go`
Add `AutoStart bool` to `CreateTaskRequest`:
```go
AutoStart bool `json:"auto_start"` // Optional: bypass project auto-start, begin immediately
```

### `api/pkg/services/spec_driven_task_service.go` — `CreateTaskFromPrompt`
After storing the task, if `req.AutoStart == true`, set the initial status to `QueuedSpecGeneration` (or `QueuedImplementation` if `JustDoItMode` is true) instead of `Backlog`. This mirrors the pattern already used by `cloneTaskToProject` in `spec_task_clone_handlers.go`.

```go
if req.AutoStart {
    if req.JustDoItMode {
        task.Status = types.TaskStatusQueuedImplementation
    } else {
        task.Status = types.TaskStatusQueuedSpecGeneration
    }
}
```

Status must be set **before** `store.CreateSpecTask` is called so the orchestrator picks it up correctly on first poll.

## Frontend Changes

### `frontend/src/api/api.ts` — `TypesCreateTaskRequest`
Add:
```ts
auto_start?: boolean;
```

### `frontend/src/components/tasks/NewSpecTaskForm.tsx`
1. Add state: `const [autoStart, setAutoStart] = useState(false);`
2. Reset in `resetForm()`: `setAutoStart(false);`
3. Include in request payload: `auto_start: autoStart`
4. Add checkbox UI below the "Just Do It" block:

```tsx
<FormControl fullWidth>
  <Tooltip title="Begin spec generation immediately on creation, overriding the project auto-start setting">
    <FormControlLabel
      control={
        <Checkbox
          checked={autoStart}
          onChange={(e) => setAutoStart(e.target.checked)}
          color="primary"
        />
      }
      label={
        <Box>
          <Typography variant="body2" sx={{ fontWeight: 600 }}>
            Start immediately
          </Typography>
          <Typography variant="caption" color="text.secondary">
            Begin spec generation on creation, regardless of project auto-start setting
          </Typography>
        </Box>
      }
    />
  </Tooltip>
</FormControl>
```

## Notes for Implementors

- Pattern for `auto_start` in clone task (`spec_task_clone_handlers.go:176`) is the reference implementation — the same status-setting logic applies here.
- The orchestrator (`spec_task_orchestrator.go:handleBacklog`) already skips tasks not in `backlog` status, so setting `QueuedSpecGeneration` status at creation is sufficient to trigger processing without touching the orchestrator.
- `TypesCreateTaskRequest` in `api.ts` is a generated file — check if there's a generator step or if it's edited manually. If generated, update the source spec (swagger/openapi) instead.
- No database migration needed: status column already supports all required values.

## Implementation Notes (added during implementation)

- **"Just Do It" UI rename**: Reviewer flagged that `Just Do It` was renamed to **`Skip planning`** in the UI label/copy. The backend Go field is still `JustDoItMode` (column `yolo_mode`), and the frontend state variable is still `justDoItMode` — only the visible label and helper text changed. Our new "Start immediately" checkbox uses `color="primary"` (blue) to visually distinguish from "Skip planning" (`color="warning"`, orange).
- **API client is generated** from swagger via `./stack update_openapi`. Don't hand-edit `frontend/src/api/api.ts`. Workflow: edit Go struct → run `./stack update_openapi` (need `swag` on PATH: `export PATH="$HOME/go/bin:$PATH"`) → field appears in `TypesCreateTaskRequest`.
- **Status logic**: Mirrors `spec_task_clone_handlers.go:176`. When `req.AutoStart && req.JustDoItMode` → `TaskStatusQueuedImplementation`; when `req.AutoStart && !req.JustDoItMode` → `TaskStatusQueuedSpecGeneration`; otherwise → `TaskStatusBacklog`. Status is set **before** `store.CreateSpecTask` so the orchestrator picks it up on first poll.
- **Build commands** that worked: backend `go build ./api/pkg/types/ ./api/pkg/services/`; frontend `cd frontend && yarn install && yarn build` (note: `yarn build` calls `vite build` which requires `node_modules` to be installed). TypeScript verification: `yarn tsc --noEmit`.
- **Files changed**:
  - `api/pkg/types/simple_spec_task.go` — added `AutoStart bool` to `CreateTaskRequest`
  - `api/pkg/services/spec_driven_task_service.go` — `CreateTaskFromPrompt` honors `AutoStart` to set initial status
  - `api/pkg/server/swagger.yaml`, `frontend/swagger/swagger.yaml`, `openapi.json`, `swagger.json`, `frontend/src/api/api.ts` — regenerated by `./stack update_openapi`
  - `frontend/src/components/tasks/NewSpecTaskForm.tsx` — new state, reset, payload field, and checkbox UI
