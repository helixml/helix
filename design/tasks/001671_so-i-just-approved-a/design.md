# Design

## Status: Already fixed on an unmerged branch

After merging latest `main` (helix-4 at `49ef585ba`), I checked the codebase and found this bug is **already fixed** on unmerged branch `feature/001920-after-approving-a-task` (commit `aa8afd883`, "After approval, jump to chat/desktop and stop the auto-open bounce", authored Fri May 8 2026 by Luke Marsden).

### What the existing branch does

Three coordinated changes:

**1. `frontend/src/lib/specTaskAutoOpen.ts` (new file)** — extracts the sessionStorage helpers (`AUTO_OPENED_KEY`, `getAutoOpenedSpecTasks`, `addAutoOpenedSpecTask`) into a shared module so both the detail page and review page can use them.

**2. `frontend/src/pages/SpecTaskReviewPage.tsx`**
- Adds a mount-time `useEffect` that calls `addAutoOpenedSpecTask(taskId)`. This means *any* way of reaching the review page (deep link, notification, breadcrumb, post-approval redirect) marks the task as already-auto-opened, so navigating back to the task detail page never re-fires the auto-open.
- Changes `handleApproved` destination from `project-specs` (kanban — where `openTask` is silently ignored) to `project-task-detail` (the chat/desktop view the user actually wants).

**3. `frontend/src/components/tasks/SpecTaskDetailContent.tsx`**
- Imports the helpers from the new shared module.
- Adds a defence-in-depth `!task?.spec_approved_at` guard to the auto-open `useEffect`. Even before React Query refreshes `task.status` away from `spec_review`, the auto-open won't fire for an already-approved task.

### Why this approach is better than the one I originally proposed

I'd suggested also modifying `DesignReviewContent.handleSubmitReview` to skip `onClose()` after `onImplementationStarted()`. The existing branch doesn't need to — once `handleApproved` and `handleBack` both navigate to `project-task-detail`, the redundant second navigation is harmless (same destination). And the `spec_approved_at` guard is more semantically correct than my "write to sessionStorage in handleApproved" idea — it covers the race condition properly.

## Recommendation

Just merge `feature/001920-after-approving-a-task` (or rebase the single commit `aa8afd883` onto current main). No new design work needed.

If a PR exists for that branch, review and merge it. If not, open one.

## Files changed by the existing fix

| File | Change |
|------|--------|
| `frontend/src/lib/specTaskAutoOpen.ts` | New shared module |
| `frontend/src/pages/SpecTaskReviewPage.tsx` | Mark task on mount + change approval destination |
| `frontend/src/components/tasks/SpecTaskDetailContent.tsx` | Use shared module + add `spec_approved_at` guard |
