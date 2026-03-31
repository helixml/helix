# Fix excessive browser notifications

## Summary

Two bugs caused users to receive far more OS-level browser notifications than expected.

1. **Double-pop per task**: When a spec task completes, `specs_pushed` and `agent_interaction_completed` events arrive together. The bell panel already collapses them into one card, but browser notifications looped over raw events — so each completion produced two OS popups. Fixed by running `newEvents` through the existing `groupEvents()` helper before firing notifications.

2. **Re-fire on navigation**: The `shownRef` set in `useBrowserNotifications` was an in-memory ref that reset on every component mount. Navigating between pages caused `GlobalNotifications` to remount, making all unacknowledged events appear "new" and re-fire. Fixed by seeding `shownRef` from `sessionStorage` on mount and writing new IDs back after each notification.

## Changes

- `frontend/src/hooks/useBrowserNotifications.ts` — persist shown notification IDs in `sessionStorage` so deduplication survives remounts within a browser session
- `frontend/src/components/system/GlobalNotifications.tsx` — group `newEvents` via `groupEvents()` + `deduplicateGroupsByTask()` before firing browser notifications; paired events produce one notification with title "Spec ready & agent finished"
