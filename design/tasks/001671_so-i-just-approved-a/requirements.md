# Requirements

## Problem

When a user approves a design from `SpecTaskReviewPage`, they briefly see the spec detail page and are then immediately bounced back to the spec review page. They want to stay on the detail page so they can watch the agent implement.

This is a frontend-only navigation bug (the backend correctly updates task status on approval).

## Root Cause

Two interacting issues:

1. `SpecTaskReviewPage.handleApproved` navigates to `project-specs` without `tab=workspace`, so the `openTask` param is silently ignored and the user lands on the kanban view (not the desired chat/desktop view). A subsequent `onClose()` call from `DesignReviewContent` then sends them to `project-task-detail` instead.
2. The auto-open `useEffect` in `SpecTaskDetailContent` (line ~933) redirects to spec review whenever `task.status` is `spec_review` and the task ID is not in sessionStorage. If the user reached the review page directly (deep link / notification / breadcrumb), the task ID was never added to sessionStorage, so this guard doesn't fire and the user gets bounced back.

## Status

**Already fixed** on unmerged branch `feature/001920-after-approving-a-task` (commit `aa8afd883`). See `design.md` for details.

## Acceptance Criteria

- After approving a design review, the user lands on the task detail page (`project-task-detail`) and stays there
- Reaching the spec review page via any path (auto-open, deep link, notification, breadcrumb) prevents the auto-open from re-firing
- Original "auto-open on first visit to a genuinely-awaiting-review task" behaviour is preserved
