# Design: Duplicate Task Detection

## Architecture

### 1. Full-Text Search Index

Add a generated `tsvector` column to `spec_tasks` and a GIN index:

```sql
ALTER TABLE spec_tasks ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(original_prompt, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(description, '')), 'C')
  ) STORED;

CREATE INDEX idx_spec_tasks_search_vector ON spec_tasks USING GIN(search_vector);
```

Since this project uses GORM AutoMigrate (no migration files), the column and index must be added via a `db.Exec()` call in the migration/setup code in `postgres.go` after AutoMigrate runs.

### 2. Duplicate Search Endpoint

**`POST /api/v1/spec-tasks/find-duplicates`**

Request:
```json
{
  "prompt": "the user's task description",
  "project_id": "proj_xxx",
  "limit": 5,
  "threshold": 0.1
}
```

Response:
```json
{
  "duplicates": [
    {
      "task": { /* SpecTask object */ },
      "score": 0.85
    }
  ]
}
```

Backend uses `plainto_tsquery('english', ?)` against the `search_vector` column, ranked by `ts_rank()`. Filters: same project, not archived.

### 3. Task Comments Model

New table `spec_task_comments` for general-purpose comments on tasks (separate from the existing design review comments):

```go
type SpecTaskComment struct {
    ID         string    `json:"id" gorm:"primaryKey;size:255"`
    SpecTaskID string    `json:"spec_task_id" gorm:"not null;size:255;index"`
    UserID     string    `json:"user_id" gorm:"not null;size:255;index"`
    Content    string    `json:"content" gorm:"type:text;not null"`
    Source     string    `json:"source" gorm:"size:50"` // "duplicate_detection", "manual"
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

Endpoints:
- `POST /api/v1/spec-tasks/{taskId}/comments` — add comment
- `GET /api/v1/spec-tasks/{taskId}/comments` — list comments

### 4. Frontend Flow

In `NewSpecTaskForm.tsx`, intercept the submit:

1. User clicks "Create task"
2. Call `POST /api/v1/spec-tasks/find-duplicates` with the prompt
3. If no matches: proceed to create as normal
4. If matches found: show a `DuplicateTaskDialog` with:
   - List of matching tasks (name, status, relevance score)
   - Each row has: "Add as comment" button and a link to view the task
   - "Create anyway" button at the bottom
5. "Add as comment" calls `POST /api/v1/spec-tasks/{taskId}/comments` with the prompt text, then closes the form
6. "Create anyway" proceeds with normal `v1SpecTasksFromPromptCreate`

## Codebase Notes

- **GORM AutoMigrate** is used for schema: the `SpecTaskComment` struct added to `postgres.go` AutoMigrate list handles table creation. The `tsvector` column and GIN index must be added via raw SQL after AutoMigrate (GORM doesn't support generated columns natively).
- **Swagger annotations** are required on new handlers so `./stack update_openapi` generates the TypeScript client methods.
- **API client pattern**: Frontend must use `api.getApiClient().v1SpecTasksFindDuplicatesCreate(...)` — never raw fetch.
- **Existing search** in `store_search.go` uses simple `LIKE` — this feature uses proper `tsvector`/`ts_rank` instead.
- The `NewSpecTaskForm.tsx` already has a submit handler that calls the API; the duplicate check inserts before that call.

## Key Decisions

- **Postgres `tsvector` over BM25 extension**: Native Postgres full-text search is "good enough" for v1 and avoids adding `pg_bm25` or similar extensions. `ts_rank` with weighted vectors (name=A, prompt=B, description=C) provides reasonable relevance ranking.
- **Separate comments table vs reusing design review comments**: Design review comments are tightly coupled to reviews (require `review_id`, `document_type`, etc.). A simple `spec_task_comments` table is cleaner and supports the general-purpose "add a note" use case.
- **POST for find-duplicates** (not GET): The prompt text can be long, so it goes in the request body.
- **Advisory only**: Duplicate detection never blocks task creation — users always have "Create anyway".
