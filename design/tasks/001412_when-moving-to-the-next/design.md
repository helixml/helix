# Design: Kanban Task Sorting on Status Change

## Current Behavior

- Tasks are sorted by `created_at DESC` in `ListSpecTasks` (store_spec_tasks.go:354)
- When status changes, only `updated_at` is modified
- Result: moved tasks stay at their original position (by creation date) in the new column

## Solution

Add a `status_updated_at` timestamp field that tracks when the status last changed. Sort Kanban columns by this field.

## Database Change

Add field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`:

```go
StatusUpdatedAt *time.Time `json:"status_updated_at,omitempty" gorm:"index"` // When status last changed
```

## Backend Changes

1. **Update handler** (`api/pkg/server/spec_driven_task_handlers.go`): Set `StatusUpdatedAt = time.Now()` when status changes
2. **Create handler**: Set `StatusUpdatedAt` to `CreatedAt` for new tasks
3. **Store query** (`api/pkg/store/store_spec_tasks.go`): Change sort to `ORDER BY status_updated_at DESC`

## Frontend Changes

None required - sorting is handled by the API response order.

## Migration

Existing tasks with `NULL` status_updated_at will sort after tasks with values. A one-time migration can set `status_updated_at = updated_at` for existing tasks if needed (optional, low priority).

## Alternatives Considered

1. **Sort by `updated_at`**: Rejected because any edit (name, description, priority) would bump the task to the top, not just status changes.
2. **Frontend-only sorting**: Rejected because it would require fetching an additional timestamp field and re-sorting client-side.