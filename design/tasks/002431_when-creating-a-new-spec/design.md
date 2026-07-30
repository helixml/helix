# Design: Navigate to Task Detail Page After Creating a Spec Task

## Current behaviour (what discovery found)

`NewSpecTaskForm` (`frontend/src/components/tasks/NewSpecTaskForm.tsx`) is the one
creation form, rendered in three places. It does the API call
(`v1SpecTasksFromPromptCreate`), applies labels, uploads attachments, invalidates
the `spec-tasks` query, resets itself, and then calls
`onTaskCreated(response.data)`. **It never navigates** — the destination is
entirely the caller's decision. Three callers:

| Caller | File | Current `onTaskCreated` |
|---|---|---|
| Kanban / audit / workspace side panel | `pages/SpecTasksPage.tsx:636` | closes panel, sets `focusTaskId` for 5s, bumps `refreshTrigger` |
| Task detail page side panel | `pages/SpecTaskDetailPage.tsx:63` | closes panel, nothing else |
| Workspace create tab | `components/tasks/TabsView.tsx:1500` | replaces the create tab with a tab for the new task |

So `TabsView` already satisfies the request; the two side-panel callers don't.

Relevant route: `org_project-task-detail` → `/orgs/:org_id/projects/:id/tasks/:taskId`
(`router.tsx:241`), rendered by `SpecTaskDetailPage`. The kanban already navigates
there on card click via `account.orgNavigate("project-task-detail", { id: projectId, taskId })`
(`SpecTasksPage.tsx:1247`). We reuse that exact call — no new route, no new helper.

## Key decision: navigate at the call site, not inside the form

`NewSpecTaskForm` stays navigation-free. Putting `orgNavigate` inside it would
either break `TabsView`'s tab behaviour or require a `shouldNavigate` prop, which
is the same conditional pushed one level down with less context. Each caller
already knows what "go to the task" means for its surface.

## Changes

### 1. `SpecTasksPage.handleTaskCreated`

```ts
const handleTaskCreated = useCallback((task: TypesSpecTask) => {
  setCreateDialogOpen(false);
  if (!task.id) return;                    // defensive: nothing to navigate to
  if (viewMode === "workspace") {
    // stay in the workspace; open the task as a tab via the existing param
    account.orgNavigate("project-specs", { id: projectId, tab: "workspace", openTask: task.id });
    return;
  }
  account.orgNavigate("project-task-detail", { id: projectId, taskId: task.id });
}, [viewMode, projectId]);
```

Notes:
- `viewMode` and `projectId` are the only dependencies — both primitives, per the
  repo's dependency-array rule. `account.orgNavigate` is deliberately *not* in the
  array (context value; `account.tsx` keeps an `organizationToolsRef` precisely so
  a stale closure still resolves the current org).
- `setRefreshTrigger` is dropped from this path: the form already invalidates the
  `spec-tasks` query, and we're leaving the board anyway.
- `handleOpenInWorkspace` in `SpecTaskDetailPage` already uses the
  `project-specs` + `tab=workspace` + `openTask` triple, so the workspace branch
  is an existing, proven pattern — `openTaskId` is read at `SpecTasksPage.tsx:178`
  and fed to `TabsView` as `initialTaskId`.

### 2. `SpecTaskDetailPage.handleTaskCreated`

```ts
const handleTaskCreated = useCallback((task: TypesSpecTask) => {
  setCreateDialogOpen(false);
  if (task.id) {
    account.orgNavigate("project-task-detail", { id: projectId, taskId: task.id });
  }
}, [projectId]);
```

Same route, different `taskId` — react-router5 treats this as a param change, so
`SpecTaskDetailPage` re-reads `route.params.taskId` and `useSpecTask` refetches.
No remount is needed; the `taskId` prop flows into `SpecTaskDetailContent`.

### 3. Dead-code cleanup: `focusTaskId`

With the kanban path navigating away, nothing sets `focusTaskId` any more. The
whole chain is then unused:

- `SpecTasksPage.tsx:335` state + `:1260` prop
- `SpecTaskKanbanBoard.tsx` `focusTaskId` prop (`:256`, `:276`, `:305`, `:673`, `:2006`, `:2052`)
  and the `focusStartPlanning={task.id === focusTaskId}` at `:351`
- `TaskCard.tsx` `focusStartPlanning` prop (`:220`, `:581`, the focus effect at
  `:625–632`, and the memo comparison at `:1713`)

`SpecTasksPage` is the only consumer of `SpecTaskKanbanBoard`, and
`SpecTaskKanbanBoard` the only consumer of `focusStartPlanning`, so the removal is
self-contained. Gated on Open Question 3 — if the answer is "keep it", leave the
props in place and simply stop setting the state.

## Ordering / async gotcha

Navigation must not race the attachment upload. In `NewSpecTaskForm.handleCreateTask`
the order is already: create → labels → `await uploadAttachments` → snackbar →
`resetForm()` → `onTaskCreated(...)`. Since `onTaskCreated` fires last, navigating
inside it cannot unmount the form mid-upload. **Do not move `onTaskCreated` earlier.**

## Things that stay as they are

- `resetForm()` clears the localStorage draft (`helix_new_spectask_draft_<projectId>`)
  before navigation, so the panel is empty next time it opens — no change needed.
- The `snackbar.success("SpecTask created! …")` toast survives the route change
  (snackbar is app-level) and is still useful confirmation on the destination page.
- Error path: `onTaskCreated` is only called inside `if (response.data)`, so a
  failed create never navigates.

## Testing plan

Per `CLAUDE.md`, verify end-to-end in the inner Helix at `http://localhost:8080`
(register `test@helix.ml` / `helixtest`, complete onboarding), not just by unit test:

1. Kanban view → "+" → create a task → assert URL becomes
   `/orgs/<org>/projects/<prj>/tasks/<task>` and the detail page renders.
2. Browser Back → returns to the board.
3. Detail page → "+" in the topbar → create → assert the URL's `taskId` changes to
   the new task and the content swaps.
4. Workspace view → side-panel create → assert we stay on `project-specs` with the
   new task open as a tab.
5. Workspace create-tab → create → assert unchanged tab-replacement behaviour.
6. Create a task with an attachment → assert the attachment is present on the
   destination page (upload wasn't cut short by navigation).

`cd frontend && yarn build` before committing. `Onboarding.test.tsx` already covers
its own navigation and must stay green.

## Notes for future agents

- **Navigation in this frontend is `account.orgNavigate('<route-without-org_-prefix>', params)`**,
  not `<Link>`/`<a href>`. `orgNavigate` prepends `org_` and resolves `org_id` from
  (in order) an explicit param → loaded org → the org slug in the URL → the first
  org in the list (`contexts/account.tsx:362`).
- Cross-surface state is passed through route params, not props: `openTask`,
  `tab`, `highlight`, `invite` are all read off `router.params` in `SpecTasksPage`.
- `SpecTaskDetailContent` is shared by the standalone page and the workspace tab —
  changes there hit both surfaces.
- One remaining raw navigation exists in `components/specTask/CloneTaskDialog.tsx`
  (`window.location.href = /projects/${projectId}` — a full page load, and a
  non-org-scoped path). Out of scope here, but worth fixing when someone touches
  that dialog.
