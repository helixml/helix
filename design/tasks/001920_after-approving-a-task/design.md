# Design

## Summary

A one-line behaviour change in `frontend/src/pages/SpecTaskReviewPage.tsx`: replace the post-approval navigation target so the user lands on the task detail page (which renders `SpecTaskDetailContent` with chat+desktop) instead of the project-specs Kanban page.

## Current Flow (frontend only)

`DesignReviewContent.handleSubmitReview` (frontend/src/components/spec-tasks/DesignReviewContent.tsx:963)
- On `approve`: calls `onImplementationStarted?.()` then `onClose()`
- On `request_changes`: calls only `onClose()`

`DesignReviewContent.handleStartImplementation` (frontend/src/components/spec-tasks/DesignReviewContent.tsx:1034) — also fires `onImplementationStarted?.()` after a successful POST to `/v1/spec-tasks/{id}/approve-implementation`.

`onImplementationStarted` has two callers:

1. **`SpecTaskReviewPage.handleApproved`** (frontend/src/pages/SpecTaskReviewPage.tsx:60) — standalone page route `/orgs/:org_id/projects/:id/tasks/:taskId/review/:reviewId`. Today it does:
   ```ts
   account.orgNavigate('project-specs', { id: projectId, openTask: taskId })
   ```
   That sends the user back to the Kanban + opens the task panel inside `SpecTasksPage`.

2. **`TabsView` inline workspace** (frontend/src/components/tasks/TabsView.tsx:1487) — closes the review tab and adds a task tab. The task tab renders `SpecTaskDetailContent`, which already defaults to the `desktop` view (chat panel alongside). **No change needed here.**

## Target Flow

Change `SpecTaskReviewPage.handleApproved` to:

```ts
const handleApproved = () => {
  // After approval the agent is implementing - jump straight to the task's
  // chat/desktop, not back to the spec list.
  account.orgNavigate('project-task-detail', { id: projectId, taskId })
}
```

Route already exists: `org_project-task-detail` → `/orgs/:org_id/projects/:id/tasks/:taskId` → `<SpecTaskDetailPage />` → `<SpecTaskDetailContent />`.

## Why `project-task-detail` and not `project-team-desktop`?

- `project-team-desktop` (`TeamDesktopPage`) is for **exploratory / Human Desktop sessions** — a different concept (shared scratch session) keyed by `sessionId`. It is **not** the per-task chat/desktop.
- `project-task-detail` renders `SpecTaskDetailContent`, which is the canonical chat+desktop view for a SpecTask. On wide viewports it shows desktop with the chat panel beside it; on mobile the default tab is `chat`. This is exactly what the user is asking for.

## View Selection

`SpecTaskDetailContent.getInitialView` (frontend/src/components/tasks/SpecTaskDetailContent.tsx:315) already picks:
- `desktop` on wide viewports (chat panel visible)
- `chat` on mobile (<900px)
- whatever `?view=` URL param overrides it to

We do **not** need to set `?view=` explicitly — the default is the right answer for both form factors. Leaving the URL clean also means a returning user's `?view=` preference (e.g. they previously chose `changes`) is not clobbered if they land here for the same task again, since this navigation has no view param.

## Files Touched

- `frontend/src/pages/SpecTaskReviewPage.tsx` — change one line in `handleApproved`. Update the comment above it.

## Files Investigated but NOT Changed

- `frontend/src/components/spec-tasks/DesignReviewContent.tsx` — callback contract (`onImplementationStarted`) is unchanged; no edit needed.
- `frontend/src/components/tasks/TabsView.tsx` — workspace flow already correct.
- `frontend/src/pages/EmbedTaskPage.tsx` — does not render `DesignReviewContent` with `onImplementationStarted`; no impact.
- API handlers (`approveImplementationHandler`, `submitReview`) — unchanged.

## Edge Cases / Notes for Future Implementer

- The existing `onClose()` call in `handleSubmitReview` (line 978) fires after `onImplementationStarted()`. Since our new handler navigates the whole page, the `onClose()` becomes effectively a no-op for the standalone page path (state on the unmounting page). Behaviour is unchanged for the `TabsView` path because `onClose` there closes the tab — which the existing inline handler already does explicitly.
- The "Back" button in the browser will return to the review URL — desirable. No `navigateReplace`.
- Snackbar success ("Design approved! Agent starting implementation...") fires before navigation, so the user sees the toast on either page (task detail page hosts the same `SnackbarProvider`).
- A previous Helix CLAUDE.md rule applies: prefer `useRouter()` / `account.orgNavigate(...)` over `<Link>` or raw `<a href>` — we are sticking with the existing pattern.

## Discoveries (for future agents)

- The "spec/Kanban" page is the route name `project-specs` (file `SpecTasksPage.tsx`). It accepts `openTask`, `openDesktop`, `openReview`, and `tab=workspace|kanban|audit` query params.
- The "task detail" page is `project-task-detail` (file `SpecTaskDetailPage.tsx` → renders `SpecTaskDetailContent`). View is controlled by `?view=chat|desktop|changes|details`.
- The "Human Desktop" page is `project-team-desktop` and is **not** the per-task chat/desktop — it is for exploratory team sessions keyed by an exploratory session ID.
- `account.orgNavigate(name, params)` is the standard way to navigate; it injects `org_id` automatically based on current org context.
- `onImplementationStarted` is the callback both the **Approve** review submission and the **Start Implementation** button fire — so changing the single callback target also covers the Start-Implementation path (AC4).
