# Requirements: Fix Spec Review Auto-Navigation Loop

## Problem

When a user is on the spec review page and clicks "Back" to return to the task detail page, they are immediately auto-redirected back to the spec review. This creates a navigation loop that makes it impossible to stay on the task detail page while the task is in `spec_review` or `spec_revision` status.

## Root Cause

`SpecTaskDetailContent.tsx` has a `useEffect` (line ~844) that auto-opens the spec review when:
- Task status is `TaskStatusSpecReview` or `TaskStatusSpecRevision`
- Design docs exist
- The task ID is NOT in `sessionStorage` key `helix_auto_opened_spec_tasks`

The guard works correctly when the user reaches the review via `handleReviewSpec` (which marks the task in sessionStorage before navigating). However, if the user arrives at the review page via **any other path** (direct URL, browser history restoration, notification link, or the page-level route), the task is never marked in sessionStorage. Navigating back to the task detail then re-triggers the auto-open.

## User Stories

- As a user on the spec review page, when I navigate away via breadcrumbs (or any other navigation), I want to land on the task detail page and stay there — not be redirected back to the review.
- As a user, I want to be able to view the task detail page for a task in `spec_review` status without being forced into the review every time.

## Acceptance Criteria

1. After navigating away from the spec review page (e.g. via breadcrumbs), the user stays on the destination page.
2. The auto-open behaviour still works on the **first** visit to the task detail page after the task transitions to `spec_review` status.
3. The fix works regardless of how the user reached the spec review page (direct URL, auto-open from task detail, or notification link).
4. The fix does not break the workspace split-screen path (`onOpenReview` callback).
