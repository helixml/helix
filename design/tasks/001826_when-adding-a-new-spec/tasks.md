# Implementation Tasks

## Backend

- [ ] Add `SpecTaskComment` model to `api/pkg/types/` with ID, SpecTaskID, UserID, Content, Source, timestamps
- [ ] Register `SpecTaskComment` in GORM AutoMigrate in `api/pkg/store/postgres.go`
- [ ] Add raw SQL migration in `postgres.go` (after AutoMigrate) to create `search_vector` generated tsvector column and GIN index on `spec_tasks`
- [ ] Add `FindDuplicateSpecTasks(ctx, projectID, prompt, limit, threshold)` method to the store layer in `api/pkg/store/`
- [ ] Add `CreateSpecTaskComment` and `ListSpecTaskComments` methods to the store layer
- [ ] Add store interface methods to `api/pkg/store/store.go`
- [ ] Add `POST /api/v1/spec-tasks/find-duplicates` handler with Swagger annotations in `api/pkg/server/`
- [ ] Add `POST /api/v1/spec-tasks/{taskId}/comments` handler with Swagger annotations
- [ ] Add `GET /api/v1/spec-tasks/{taskId}/comments` handler with Swagger annotations
- [ ] Run `./stack update_openapi` to regenerate the TypeScript API client

## Frontend

- [ ] Create `DuplicateTaskDialog` component showing matching tasks with scores, "Add as comment" and "Create anyway" actions
- [ ] Modify `NewSpecTaskForm.tsx` submit handler to call find-duplicates before creating, show dialog if matches found
- [ ] Add comment creation call when user selects "Add as comment on duplicate"
- [ ] Show success feedback after adding comment to duplicate (snackbar + close form)

## Testing

- [ ] Verify `go build ./api/...` passes
- [ ] Verify `cd frontend && yarn build` passes
- [ ] Test end-to-end in browser: create task with similar prompt to existing task, verify dialog appears
