# Design: Labels for Spec Tasks

## Existing State

`SpecTask` in `api/pkg/types/simple_spec_task.go` already has:
```go
Labels []string `json:"labels" gorm:"type:jsonb;default:'[]'"`
```
The column exists in PostgreSQL. No migration is needed.

## Store Layer (`api/pkg/store/store_spec_tasks.go`)

Add three methods to the store interface and implementation:

```go
// Return unique labels across all tasks in a project (sorted).
ListProjectLabels(ctx context.Context, projectID string) ([]string, error)

// Add a label to a task (no-op if already present).
AddSpecTaskLabel(ctx context.Context, taskID string, label string) error

// Remove a label from a task (no-op if not present).
RemoveSpecTaskLabel(ctx context.Context, taskID string, label string) error
```

`ListProjectLabels` uses a raw SQL query (or GORM raw) to unnest the JSONB array and return distinct values:
```sql
SELECT DISTINCT jsonb_array_elements_text(labels) AS label
FROM spec_tasks
WHERE project_id = ? AND archived = false
ORDER BY label;
```

`AddSpecTaskLabel` and `RemoveSpecTaskLabel` load the task, mutate the slice, and save — keeping it simple with GORM.

**Filter change:** `ListSpecTasks` already accepts a filter struct. Add a `Labels []string` field; when non-empty, add a GORM condition:
```go
// matches tasks that have ALL specified labels
for _, l := range filters.Labels {
    db = db.Where("labels @> ?", `["`+l+`"]`)
}
```

## API Layer (`api/pkg/server/`)

Add a new handler file `spec_task_label_handlers.go` with three endpoints registered in the router:

```
GET    /api/v1/projects/{projectId}/labels               → listProjectLabels
POST   /api/v1/spec-tasks/{taskId}/labels                → addLabel   (body: {"label":"..."})
DELETE /api/v1/spec-tasks/{taskId}/labels/{label}        → removeLabel
```

The existing `GET /api/v1/spec-tasks` handler gains a `labels` query param (comma-separated).

All handlers follow existing patterns: `getRequestUser`, `authorizeUserToProjectByID`, JSON response.

## Frontend (`frontend/src/`)

**New service methods in `specTaskService.ts`:**
- `useProjectLabels(projectId)` — fetches `GET /projects/{id}/labels`
- `useAddLabel(taskId)` — mutation for `POST /spec-tasks/{id}/labels`
- `useRemoveLabel(taskId)` — mutation for `DELETE /spec-tasks/{id}/labels/{label}`

**Task detail view** – add a labels section:
- Displays existing labels as MUI `Chip` components with a delete icon.
- Autocomplete input that suggests labels from `useProjectLabels`, allows free entry, adds on Enter/comma.

**Task list view** – add a label filter:
- Multi-select autocomplete above (or beside) existing filters.
- Selected labels are passed as `labels=a,b` query param to the API.
- Label chips shown on each task card in the list.

## Key Decisions

- **No separate `labels` table.** The JSONB column is sufficient for free-form string tags and avoids a join. If label metadata (color, description) is needed later it can be added as a separate step.
- **ALL semantics for multi-label filter.** Tasks must have every selected label (AND), not any (OR). This is the most common expectation for tag-based filtering.
- **Idempotent add.** Calling add with an existing label is a no-op; no error returned.
- **Raw SQL for `ListProjectLabels`.** GORM doesn't provide a clean way to unnest JSONB arrays; a single raw query is cleaner than Go-side aggregation.
- **Reuse existing update endpoint for bulk set.** The existing `PUT /api/v1/spec-tasks/{taskId}` already accepts `labels` in the body. The new endpoints are for incremental add/remove without having to send the full task payload.
