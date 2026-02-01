# Implementation Tasks

## Backend

- [ ] Add `sort_order` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [ ] Run GORM AutoMigrate to add column (happens automatically on startup)
- [ ] Add `PATCH /api/v1/spec-tasks/{id}` support for updating `sort_order` (may already exist via `SpecTaskUpdateRequest`)
- [ ] Add batch reorder endpoint `POST /api/v1/projects/{id}/spec-tasks/reorder` that accepts `{ task_ids: string[] }`
- [ ] Update `ListSpecTasks` query to order by `sort_order ASC, created_at DESC` for backlog tasks
- [ ] Add swagger annotations and run `./stack update_openapi`

## Frontend

- [ ] Create `BacklogTableView.tsx` component in `frontend/src/components/tasks/`
- [ ] Add table icon button to backlog column header in `SpecTaskKanbanBoard.tsx`
- [ ] Implement MUI Dialog with full-screen table layout
- [ ] Add table columns: drag handle, task number, name, prompt (expandable), priority, created, status
- [ ] Implement expandable rows for full prompt text (follow `ProjectAuditTrail.tsx` pattern)
- [ ] Add `@dnd-kit` sortable rows (follow `RobustPromptInput.tsx` pattern)
- [ ] Implement priority inline edit via clickable chip + dropdown
- [ ] Call reorder API on drag-end
- [ ] Add React Query mutation for reorder with optimistic updates
- [ ] Invalidate spec-tasks query after reorder

## Testing

- [ ] Verify table opens from backlog column
- [ ] Verify drag-and-drop reorders rows visually
- [ ] Verify order persists after page refresh
- [ ] Verify priority changes save correctly
- [ ] Run `cd frontend && yarn test && yarn build` before committing