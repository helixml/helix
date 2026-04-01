# Requirements

## Problem

When a user approves a design from `SpecTaskReviewPage`, they briefly see the spec detail page and are then immediately bounced back to the spec review page.

This is a frontend-only navigation bug (the backend correctly updates task status to `spec_approved` via the `approve-specs` call).

## Root Cause

Two bugs interact:

**Bug 1 — conflicting navigation calls in `DesignReviewContent.handleSubmitReview`:**

After approval, it calls `onImplementationStarted()` (which navigates to `project-specs`) and then immediately calls `onClose()` (which navigates to `project-task-detail`). The second navigation wins, sending the user to `project-task-detail`.

**Bug 2 — auto-open `useEffect` in `SpecTaskDetailContent` fires on arrival:**

`SpecTaskDetailContent` (line 844) redirects to spec review whenever:
- task status is `spec_review` or `spec_revision`, AND
- the task ID is not in sessionStorage (the "already auto-opened" guard)

If the user navigated directly to the spec review URL (e.g. via a link/email) rather than through the normal auto-open flow, the task ID was never added to sessionStorage. So when they arrive at `project-task-detail` after approving, the effect fires and sends them back to spec review.

## Desired Behavior

After approving a design, the user should stay on the spec detail page (`project-task-detail`) so they can watch the agent begin implementation.

## Acceptance Criteria

- Approving a design review navigates the user to the spec detail page (`project-task-detail`) and keeps them there
- The auto-open `useEffect` does not redirect back to spec review after an approval
- Navigating directly to a spec review URL, approving, and being sent to the task detail page all works correctly
