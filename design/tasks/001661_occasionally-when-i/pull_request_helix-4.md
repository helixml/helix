# Fix spec review auto-navigation loop

## Summary

When a user navigated away from the spec review page (e.g. via breadcrumbs), they were immediately auto-redirected back to it. The auto-open `useEffect` in `SpecTaskDetailContent` guards against re-triggering using a sessionStorage key, but that key was only written when the review was opened via `handleReviewSpec`. If the user reached the review through any other path (direct URL, browser history, notification link), the key was never set, so navigating back to the task detail always re-triggered the auto-open.

## Changes

- `frontend/src/lib/specTaskAutoOpen.ts` — new shared file with the sessionStorage helpers (`getAutoOpenedSpecTasks`, `addAutoOpenedSpecTask`)
- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — import helpers from shared file instead of defining inline
- `frontend/src/pages/SpecTaskReviewPage.tsx` — call `addAutoOpenedSpecTask` on mount so the guard fires correctly regardless of how the user arrived at the review page
