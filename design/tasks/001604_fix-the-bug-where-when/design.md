# Design: Fix SpectTask Label Persistence Bug

## Key Files

- `api/pkg/store/store_spec_tasks.go` — `AddSpecTaskLabel()` (line ~452), `RemoveSpecTaskLabel()` (line ~476)
- `frontend/src/components/tasks/NewSpecTaskForm.tsx` — label submission logic (line ~338)
- `api/pkg/types/simple_spec_task.go` — `CreateTaskRequest` struct (line ~48)

## Chosen Approach: Fix Frontend to Add Labels Sequentially

The simplest fix is to change the frontend from concurrent (`Promise.all`) to sequential label addition during task creation. This avoids the race condition with zero backend changes.

```typescript
// Before (concurrent — causes race):
await Promise.all(taskLabels.map(label => addLabelMutation.mutateAsync({ taskId, label })));

// After (sequential — safe):
for (const label of taskLabels) {
  await addLabelMutation.mutateAsync({ taskId, label });
}
```

### Why Not Fix the Backend?

The atomic SQL approach (`jsonb_set` in a single UPDATE) is more robust long-term but requires more care:
- The `SpecTask` is stored as a GORM model with a `Labels` field (likely `datatypes.JSONType` or `pq.StringArray`)
- Changing to raw SQL for just this operation is an inconsistency
- Sequential frontend fix is safe, simple, and consistent with how other sequential operations work in the codebase

### Alternative Considered: Add Labels to CreateTaskRequest

Adding a `Labels []string` field to `CreateTaskRequest` and saving them during task creation would also fix this. However, it requires changes to the API handler, store, and type — more scope than needed.

## Notes for Future Agents

- The `AddSpecTaskLabel` backend function has an inherent TOCTOU (time-of-check/time-of-use) race — if concurrent label additions are ever needed again, the fix is an atomic SQL UPDATE using `jsonb_set` or array operators.
- The `SpecTask.Labels` field uses `datatypes.JSONType` from GORM — check actual column type before writing raw SQL.
- Pattern: this project uses `api.getApiClient()` generated client — never raw fetch.
