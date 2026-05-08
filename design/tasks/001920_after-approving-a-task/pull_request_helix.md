# After approval, jump to chat/desktop and stop the auto-open bounce

## Summary

After a user approves a spec design from the standalone Spec Review page, the app now lands them on the task's chat/desktop view (where the agent is implementing) instead of bouncing them back to the project Kanban — and stays there.

Two changes were needed to make this stick. The navigation target alone was cosmetic: the existing auto-open spec-review `useEffect` in `SpecTaskDetailContent` would re-redirect the user back to the spec the moment they landed on the task detail page, because `task.status` is still cached at `spec_review` for a brief window after approval.

## Changes

- **`frontend/src/pages/SpecTaskReviewPage.tsx`**:
  - `handleApproved` now navigates to `project-task-detail` (chat/desktop) instead of `project-specs?openTask=...` (Kanban).
  - On mount, the page writes the task ID to the `helix_auto_opened_spec_tasks` sessionStorage dedupe set, so the auto-open useEffect on the task detail page never re-bounces the user — regardless of how they reached the review page (deep link, notification, breadcrumb, or this new redirect).
- **`frontend/src/lib/specTaskAutoOpen.ts`** (new): Extracts `getAutoOpenedSpecTasks` / `addAutoOpenedSpecTask` / `AUTO_OPENED_KEY` so both the review page and the detail content reference the same key.
- **`frontend/src/components/tasks/SpecTaskDetailContent.tsx`**:
  - Imports the helpers from the new shared module; deletes the inline copies.
  - Auto-open useEffect now also bails when `task.spec_approved_at` is set (defence-in-depth — the server sets this synchronously inside the approval handler, so it's a reliable signal even before the cached `task.status` refreshes through `spec_approved` → `implementation_queued`).

The same `onImplementationStarted` callback is fired by both the Approve submission and the Start Implementation button on the review page, so both paths land on the chat/desktop. The in-workspace `TabsView` flow is unchanged (it already replaces the review tab with a task tab in-place). The original "auto-open on first visit to a genuinely-awaiting-review task" behaviour is preserved.

## Test Plan

- TypeScript: `npx tsc --noEmit` clean (no type errors). `yarn build` transforms 21093 modules; only the dist write fails in this dev env due to a read-only bind mount, which is environment, not the code.
- **WARNING: NOT manually verified in browser** — the inner Helix sandbox was still building Rust/Zed dependencies during this session. Reviewer should walk through the manual verification steps in `tasks.md` (approve from review page → land on task detail → no bounce; Back button still returns to review URL; Request Changes still closes only; first-visit auto-open for a fresh `spec_review` task still works).

## Spec Reference

`helix-specs@HEAD:001920_after-approving-a-task` — see `design.md` for the full root-cause analysis and the coordination note with task 001661 (which this PR supersedes since 001661 is unmerged on `feature/001661-occasionally-when-i`).
