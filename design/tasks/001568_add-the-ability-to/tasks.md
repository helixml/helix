# Implementation Tasks

## Store

- [x] Add `ListProjectLabels(ctx, projectID) ([]string, error)` to store interface and `store_spec_tasks.go` using raw SQL to unnest the JSONB `labels` column
- [x] Add `AddSpecTaskLabel(ctx, taskID, label) error` to store interface and implementation (load task, append if missing, save)
- [x] Add `RemoveSpecTaskLabel(ctx, taskID, label) error` to store interface and implementation (load task, filter out label, save)
- [x] Add `Labels []string` to `ListSpecTasksFilter` struct and apply JSONB containment filter (`labels @> ?`) in `ListSpecTasks` for each selected label

## API

- [~] Create `api/pkg/server/spec_task_label_handlers.go` with `listProjectLabels`, `addLabel`, and `removeLabel` handlers (with Swagger annotations)
- [ ] Register the three new routes in the router: `GET /api/v1/projects/{projectId}/labels`, `POST /api/v1/spec-tasks/{taskId}/labels`, `DELETE /api/v1/spec-tasks/{taskId}/labels/{label}`
- [ ] Add `labels` query param support to the existing `listSpecTasks` handler (parse comma-separated string, pass to filter)
- [ ] Regenerate OpenAPI/Swagger spec (`make generate-swagger` or equivalent) and commit the updated `swagger.json` / `openapi.json`
- [ ] Regenerate the TypeScript API client in `frontend/src/api/`

## Frontend

- [ ] Add `useProjectLabels`, `useAddLabel`, and `useRemoveLabel` hooks to `frontend/src/services/specTaskService.ts`
- [ ] Add label query key to `QUERY_KEYS` and invalidate `specTask` + `specTasks` on label mutation
- [ ] Build label editor component on task detail view: display existing labels as deletable MUI `Chip` elements, add an autocomplete input that suggests from `useProjectLabels` and allows free entry
- [ ] Add label filter control to task list view (multi-select autocomplete) that appends `labels=a,b` to the list query
- [ ] Display label chips on task cards in the list view
