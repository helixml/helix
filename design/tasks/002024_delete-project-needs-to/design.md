# Design: Synchronous Project Delete with Visible Spinner

## TL;DR

The bug is **not** that the backend is async — it isn't. The bug is a
**React Query cache-key mismatch** in `useDeleteProject` that leaves stale data
in every page using `useListProjects(orgId)`. The user sees the deleted project
because the cache for their org was never invalidated. Fix the invalidation,
await a refetch before navigating, and prevent the dialog from closing while
the mutation is in flight.

## Code Trace (Current State)

### Frontend

`frontend/src/services/projectService.ts:6`
```ts
export const projectsListQueryKey = (orgId?: string) => ['projects', orgId];
```

`frontend/src/services/projectService.ts:93-107` — `useDeleteProject`:
```ts
return useMutation({
  mutationFn: async (projectId: string) => {
    const response = await apiClient.v1ProjectsDelete(projectId);
    return response.data;
  },
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    //                                          ^^^^^^^^^^^^^^^^^^^^^
    //  Produces ['projects', undefined]  ← BUG
  },
});
```

The list pages, however, query with the real org id:

- `frontend/src/pages/Projects.tsx:111` →
  `useListProjects(account.organizationTools.organization?.id || "", …)`
- `frontend/src/pages/Home.tsx:155`, `Jobs.tsx:327`, `GitRepoDetail.tsx:153`,
  `components/app/ProjectManagerSkill.tsx:70` — all pass the real org id.

So the live cache keys look like `['projects', 'org_01k…']`, and the
invalidation key is `['projects', undefined]`. TanStack Query's
`invalidateQueries` does prefix matching, but `undefined` at index 1 is a
literal value that does **not** match `'org_01k…'`. Result: the cache for the
user's actual org is never invalidated, the list keeps showing the deleted
project, and the user is fooled into thinking the delete didn't happen.

`frontend/src/pages/ProjectSettings.tsx:733-747` — handler:
```ts
const handleDeleteProject = async () => {
  if (deleteConfirmName !== project?.name) { … return; }
  try {
    await deleteProjectMutation.mutateAsync(projectId);
    snackbar.success("Project deleted successfully");
    setDeleteDialogOpen(false);
    account.orgNavigate("projects");
  } catch (err) {
    snackbar.error("Failed to delete project");
  }
};
```

`frontend/src/pages/ProjectSettings.tsx:2019-2088` — dialog:
- Already shows `<CircularProgress size={16} />` and `"Deleting…"` while
  `deleteProjectMutation.isPending` is true.
- BUT the `<Dialog onClose={…}>` (lines 2022-2025) lets the user dismiss
  the dialog (backdrop click / Escape) even while the mutation is pending.
- The **Cancel** button (lines 2061-2068) is *not* disabled while pending.

### Backend (for context — not changed by this task)

`api/pkg/server/project_handlers.go:753-833` (`deleteProject`):
- Synchronously authorizes the user.
- Synchronously calls `StopDesktop` for the exploratory session.
- Synchronously calls `StopDesktop` for every spec-task's planning session
  (loops over `s.Store.ListSpecTasks(...)`).
- Synchronously calls `s.Store.DeleteProject(...)` (GORM soft delete — sets
  `deleted_at`).
- Then returns `200`. No `go func`, no background work.

So the call IS synchronous. It can be slow if the project has many running
spec-task sessions (each `StopDesktop` waits for the container to stop), but
that's fine — we *want* the user to wait. The fix is making the UI honestly
wait too.

## Fix

### 1. `useDeleteProject` — invalidate every cache key the list pages use

In `frontend/src/services/projectService.ts:93-107`, change `onSuccess` to:

```ts
onSuccess: async () => {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ['projects'] }),
    queryClient.invalidateQueries({ queryKey: ['pinned-projects'] }),
  ]);
},
```

Notes:
- `['projects']` (one-element key) **does** prefix-match every
  `['projects', orgId]` variant, including `['projects', undefined]`.
- `'pinned-projects'` is invalidated too — `usePinnedProjectIds` lives in the
  sidebar and would otherwise still show the deleted project pinned.
- `await` is important: the mutation's `mutateAsync` only resolves once
  `onSuccess` resolves, so the caller's spinner stays on through the refetch.

### 2. `ProjectSettings.tsx` — make the dialog truly modal during the mutation

In `frontend/src/pages/ProjectSettings.tsx:2019-2088`:

- Change `onClose` to a no-op while pending:
  ```tsx
  onClose={() => {
    if (deleteProjectMutation.isPending) return;
    setDeleteDialogOpen(false);
    setDeleteConfirmName("");
  }}
  ```
- Disable the **Cancel** button while pending:
  ```tsx
  <Button
    onClick={() => { setDeleteDialogOpen(false); setDeleteConfirmName(""); }}
    disabled={deleteProjectMutation.isPending}
  >Cancel</Button>
  ```

The Delete button is already disabled and already shows the spinner while
pending — no change needed there.

### 3. `handleDeleteProject` — leave the order alone

Once #1 is in place, `await deleteProjectMutation.mutateAsync(projectId)`
naturally awaits the refetch too. The existing
`setDeleteDialogOpen(false)` → `account.orgNavigate("projects")` order is
correct — the list page will mount with fresh data.

Do **not** wrap the refetch with a `setTimeout` or any other delay.

## Why Not Refactor the Backend?

The backend handler is already synchronous and is the right shape. Spawning
goroutines for cleanup would re-introduce exactly the bug the user is
complaining about. Leave it.

## Risk / Edge Cases

- **Error path.** If the mutation rejects (network error, 500), `onSuccess`
  doesn't run, so caches aren't invalidated. That's the correct behaviour —
  the project wasn't deleted. The dialog stays open via `catch`, and the user
  can retry or cancel.
- **Concurrent delete from another tab.** If the backend returns 404 (already
  deleted), the mutation rejects → the snackbar says "Failed to delete
  project". Acceptable; would only happen with concurrent admin action.
- **Many spec-task sessions.** The handler can take several seconds because
  it stops each session synchronously. The spinner now correctly stays on for
  that duration — this is the user's stated preference.

## Manual Test Plan

1. Create a project in an org.
2. Open project settings → Danger Zone → Delete Project.
3. Type the name, click **Delete Project**.
4. **Observe**: spinner spins, Cancel is disabled, dialog cannot be dismissed
   by clicking outside or pressing Escape.
5. **Observe**: when the spinner stops, the dialog closes and you land on the
   projects list. The deleted project is **not** there.
6. **Sidebar check**: if the project was pinned, the pin disappears too.
7. **Error path**: temporarily kill the API container, repeat — confirm the
   dialog stays open and a "Failed to delete project" toast appears.

## Learnings to Carry Forward

- **TanStack Query gotcha**: `invalidateQueries({ queryKey: ['x', undefined] })`
  does NOT match `['x', 'real-value']`. Either pass the real value, or pass a
  shorter prefix (`['x']`). The same pattern bug very likely exists in other
  hooks that call `projectsListQueryKey()` with no argument — worth grepping
  during implementation (e.g. `useUpdateProject` at line 85 has the same shape
  and should be inspected, even if it isn't part of this task's fix).
- **Established pattern** for "await refetch before resolving":
  `useStartProjectExploratorySession` (projectService.ts:269-274) already
  does `await queryClient.refetchQueries(...)` in `onSuccess`. Reuse this
  style for any mutation whose UI flow depends on fresh data.
- **Material-UI Dialog**: `onClose` fires on backdrop/Escape regardless of the
  buttons inside. If you want a dialog that cannot be dismissed during an
  operation, gate `onClose` on the in-flight state — disabling buttons is
  not enough.
