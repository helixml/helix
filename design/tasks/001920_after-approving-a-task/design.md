# Design

## Summary

Two coordinated changes in `frontend/`:

1. Change the post-approval navigation target so the user lands on the task chat/desktop, not the project Kanban.
2. Stop the auto-open spec-review useEffect in `SpecTaskDetailContent` from bouncing the user back to the review page after they've already been to it (root-cause fix for the "useEffect that jumps you to the spec in other cases" the user flagged).

These must ship together: change (1) without (2) is cosmetic — the useEffect would re-redirect within ~1 frame.

## The Auto-Open Bug (root cause)

`SpecTaskDetailContent.tsx:921-932` (current `main`):

```ts
useEffect(() => {
  if (
    task?.id &&
    !getAutoOpenedSpecTasks().has(task.id) &&
    task?.design_docs_pushed_at &&
    account.organizationTools.organization?.name &&
    (task?.status === TypesSpecTaskStatus.TaskStatusSpecReview ||
     task?.status === TypesSpecTaskStatus.TaskStatusSpecRevision)
  ) {
    handleReviewSpec();
  }
}, [task?.id, task?.status, task?.design_docs_pushed_at, handleReviewSpec, account.organizationTools.organization?.name]);
```

`handleReviewSpec` (line 884) marks the task in `sessionStorage` (`addAutoOpenedSpecTask(task.id)`) before navigating to the review page. This dedupe is **only** written here. Failure modes:

- User reaches the review page via deep link / notification / breadcrumb / our post-approval redirect → dedupe never written.
- User then lands on (or is sent to) the task detail page while `task.status` is still cached at `spec_review` (React Query invalidation hasn't refetched yet, or the server hasn't transitioned the status yet) → useEffect fires → user is re-redirected to the spec.

There is already an unmerged fix at commit `776bc2ad4` (branch `feature/001661-occasionally-when-i`, spec ref `001661_occasionally-when-i`) that:

- Extracts the helpers into `frontend/src/lib/specTaskAutoOpen.ts`.
- Calls `addAutoOpenedSpecTask(taskId)` on `SpecTaskReviewPage` mount.

Our task **absorbs** that fix: do the same edits as part of this PR, since (a) they fix our problem and (b) we're touching `SpecTaskReviewPage.tsx` anyway.

We additionally add a defence-in-depth check: also bail out of the useEffect if `task.spec_approved_at` is set. This protects the post-approval window even before the React Query cache refreshes the status field.

## Current Flow (frontend only)

`DesignReviewContent.handleSubmitReview` (frontend/src/components/spec-tasks/DesignReviewContent.tsx:963)
- On `approve`: calls `onImplementationStarted?.()` then `onClose()`.
- On `request_changes`: calls only `onClose()`.

`DesignReviewContent.handleStartImplementation` (line 1034) — also fires `onImplementationStarted?.()` after a successful POST to `/v1/spec-tasks/{id}/approve-implementation`.

`onImplementationStarted` has two callers:

1. **`SpecTaskReviewPage.handleApproved`** (frontend/src/pages/SpecTaskReviewPage.tsx:60) — standalone page route `/orgs/:org_id/projects/:id/tasks/:taskId/review/:reviewId`. Today it does:
   ```ts
   account.orgNavigate('project-specs', { id: projectId, openTask: taskId })
   ```
   That sends the user to the Kanban (default tab; `openTask` is only consumed by `TabsView` when `tab=workspace`, so it's effectively ignored here).

2. **`TabsView` inline workspace** (frontend/src/components/tasks/TabsView.tsx:1487) — closes the review tab and adds a task tab. The task tab renders `SpecTaskDetailContent`, which already defaults to the `desktop` view (chat panel alongside). **No change needed here.**

## Target Flow

### Change 1 — `SpecTaskReviewPage.handleApproved`

```ts
const handleApproved = () => {
  // After approval the agent is implementing - jump straight to the task's
  // chat/desktop, not back to the spec list.
  account.orgNavigate('project-task-detail', { id: projectId, taskId })
}
```

Route already exists: `org_project-task-detail` → `/orgs/:org_id/projects/:id/tasks/:taskId` → `<SpecTaskDetailPage />` → `<SpecTaskDetailContent />`. Default view is `desktop` on wide viewports and `chat` on mobile.

### Change 2 — Extract sessionStorage helpers to a shared lib

New file `frontend/src/lib/specTaskAutoOpen.ts` (matches the existing fix in `776bc2ad4`):

```ts
export const AUTO_OPENED_KEY = "helix_auto_opened_spec_tasks";

export const getAutoOpenedSpecTasks = (): Set<string> =>
  new Set(JSON.parse(sessionStorage.getItem(AUTO_OPENED_KEY) || "[]"));

export const addAutoOpenedSpecTask = (id: string): void => {
  const set = getAutoOpenedSpecTasks();
  set.add(id);
  sessionStorage.setItem(AUTO_OPENED_KEY, JSON.stringify([...set]));
};
```

Update `SpecTaskDetailContent.tsx` to import from this file and delete the inline copies.

### Change 3 — Mark task as auto-opened on `SpecTaskReviewPage` mount

```ts
useEffect(() => {
  if (taskId) addAutoOpenedSpecTask(taskId)
}, [taskId])
```

Effect: any user who lands on the review page (deep link, notification, breadcrumb, post-approval redirect) is recorded in sessionStorage. When they navigate to the task detail, the auto-open useEffect's `getAutoOpenedSpecTasks().has(task.id)` check is true → no redirect.

### Change 4 — Defence-in-depth: skip auto-open for already-approved tasks

In `SpecTaskDetailContent.tsx:921` useEffect, add a `!task?.spec_approved_at` guard:

```ts
if (
  task?.id &&
  !getAutoOpenedSpecTasks().has(task.id) &&
  !task?.spec_approved_at &&
  task?.design_docs_pushed_at &&
  account.organizationTools.organization?.name &&
  (task?.status === TypesSpecTaskStatus.TaskStatusSpecReview ||
   task?.status === TypesSpecTaskStatus.TaskStatusSpecRevision)
) {
  handleReviewSpec();
}
```

Add `task?.spec_approved_at` to the dependency array. Rationale: the server sets `spec_approved_at` synchronously inside the approval handler, so even if `task.status` hasn't transitioned through `spec_approved` → `implementation_queued` yet, the presence of `spec_approved_at` is a reliable signal that the user is past the review point.

## Why `project-task-detail` and not `project-team-desktop`?

- `project-team-desktop` (`TeamDesktopPage`) is for **exploratory / Human Desktop sessions** — a different concept (shared scratch session) keyed by a separate `sessionId`. It is **not** the per-task chat/desktop.
- `project-task-detail` renders `SpecTaskDetailContent`, which is the canonical chat+desktop view for a SpecTask. On wide viewports it shows desktop with the chat panel beside it; on mobile the default tab is `chat`. This is what the user is asking for.

## View Selection

`SpecTaskDetailContent.getInitialView` (line 315) already picks:
- `desktop` on wide viewports (chat panel visible)
- `chat` on mobile (<900px)
- whatever `?view=` URL param overrides it to

We do **not** set `?view=` explicitly — the default is correct for both form factors and we don't want to clobber a user's previous `?view=` preference for the same task.

## Files Touched

- `frontend/src/pages/SpecTaskReviewPage.tsx` — change one line in `handleApproved`; add `useEffect` to mark task as auto-opened on mount; add import.
- `frontend/src/lib/specTaskAutoOpen.ts` — new file (sessionStorage helpers).
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — replace inline helpers with import; add `!task?.spec_approved_at` guard and dep-array entry.

## Files Investigated but NOT Changed

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — callback contract (`onImplementationStarted`) is unchanged.
- `frontend/src/components/tasks/TabsView.tsx` — workspace flow already correct.
- `frontend/src/pages/EmbedTaskPage.tsx` — does not render `DesignReviewContent` with `onImplementationStarted`; no impact.
- API handlers (`approveImplementationHandler`, `submitReview`) — unchanged.

## Coordination with task 001661

Task 001661 (`feature/001661-occasionally-when-i`) addresses the same useEffect bounce. Two scenarios:

- **If 001661 lands first**: drop Changes 2 and 3 from this task; keep Changes 1 and 4. Verify by checking that `frontend/src/lib/specTaskAutoOpen.ts` exists on `main` and `SpecTaskReviewPage` already calls `addAutoOpenedSpecTask(taskId)` on mount.
- **If this task lands first**: 001661 becomes a no-op or a small follow-up. The implementer of 001661 should rebase and verify their fix is already present.

The implementer of this task should check `git log origin/main -- frontend/src/lib/specTaskAutoOpen.ts` before starting and adjust scope accordingly.

## Edge Cases / Notes for Future Implementer

- The `onClose()` call in `handleSubmitReview` (DesignReviewContent.tsx:978) fires after `onImplementationStarted()`. Since our new handler navigates the whole page, `onClose()` becomes effectively a no-op for the standalone page path (state on the unmounting page). Behaviour is unchanged for the `TabsView` path because `onClose` there closes the tab — which the existing inline handler already does explicitly.
- The "Back" button in the browser will return to the review URL — desirable. No `navigateReplace`. The auto-open dedupe covers the bounce-prevention because the task ID was already added on the first review-page mount; a second mount is a no-op.
- Snackbar success ("Design approved! Agent starting implementation...") fires before navigation, so the user sees the toast on either page (task detail page hosts the same `SnackbarProvider`).
- A previous Helix CLAUDE.md rule applies: prefer `useRouter()` / `account.orgNavigate(...)` over `<Link>` or raw `<a href>` — we are sticking with the existing pattern.
- React useEffect dep arrays: per CLAUDE.md, only include primitives. `task?.spec_approved_at` is a string-or-undefined primitive — fine to add.

## Discoveries (for future agents)

- The "spec/Kanban" page is the route name `project-specs` (file `SpecTasksPage.tsx`). It accepts `openTask`, `openDesktop`, `openReview`, and `tab=workspace|kanban|audit` query params. `openTask` is only consumed by `TabsView` (`tab=workspace`); in default Kanban mode it's ignored.
- The "task detail" page is `project-task-detail` (file `SpecTaskDetailPage.tsx` → renders `SpecTaskDetailContent`). View is controlled by `?view=chat|desktop|changes|details`.
- The "Human Desktop" page is `project-team-desktop` and is **not** the per-task chat/desktop — it is for exploratory team sessions keyed by an exploratory session ID.
- `account.orgNavigate(name, params)` is the standard way to navigate; it injects `org_id` automatically based on current org context.
- `onImplementationStarted` is the callback both the **Approve** review submission and the **Start Implementation** button fire — so changing the single callback target also covers the Start-Implementation path.
- The `helix_auto_opened_spec_tasks` sessionStorage key is the dedupe set for the auto-open useEffect. `sessionStorage` is per-tab, cleared when the tab closes — exactly the right scope (a fresh tab should re-trigger the auto-open for unapproved tasks).
- Status flow after approval: `spec_review` → `spec_approved` → `implementation_queued` → `implementation`. `spec_approved_at` is set synchronously inside the approval HTTP handler, so it is the most reliable post-approval signal short of waiting for orchestrator transitions.
