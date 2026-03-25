# Design: Reduce Excessive Browser Notifications

## Architecture Notes

Browser notifications flow through two hooks and one component:

- `useAttentionEvents` (`frontend/src/hooks/useAttentionEvents.ts`) — polls `/api/v1/attention-events?active=true` every 10 s. Computes `newEvents` (events not yet in `prevEventIdsRef`) on each render.
- `useBrowserNotifications` (`frontend/src/hooks/useBrowserNotifications.ts`) — wraps the Web Notifications API. Has `shownRef` to skip duplicate IDs within a single mount lifetime.
- `GlobalNotifications` (`frontend/src/components/system/GlobalNotifications.tsx`) — groups events visually via `groupEvents()` / `deduplicateGroupsByTask()`, then fires browser notifications for every item in `newEvents` (ungrouped).

## Bug 1 — Notifications Not Grouped

`newEvents` contains raw events. The `useEffect` in `GlobalNotifications` loops over them and calls `fireNotification` per event. The `groupEvents()` helper already knows how to collapse `specs_pushed` + `agent_interaction_completed` pairs — it just isn't used for browser notifications.

**Fix**: Before the browser-notification loop, run `newEvents` through `groupEvents()` and fire one notification per group (using the primary event's ID and a merged title like "Spec ready & agent finished").

## Bug 2 — In-Memory Deduplication Resets on Remount

Both `shownRef` (in `useBrowserNotifications`) and `prevEventIdsRef` (in `useAttentionEvents`) are `useRef` values that start empty on every mount. If the layout component remounts (navigation, HMR, React StrictMode double-mount), all active events appear "new" and notifications re-fire.

**Fix**: Persist the set of shown notification IDs in `sessionStorage` (clears on tab close, which is the right scope). On mount, load the set from sessionStorage; on each `fireNotification` call, write the new ID back. This makes deduplication survive remounts within the same browser session.

`shownRef` in `useBrowserNotifications` already uses the event `id` as a key, so extending it to read/write sessionStorage is a minimal change confined to that hook.

## Key Decision

- **sessionStorage vs localStorage**: sessionStorage is preferred — old notification IDs from days ago don't need to be remembered forever, and the set doesn't grow unboundedly.
- **No server-side changes needed**: All fixes are frontend-only.
- **Grouping logic reuse**: `groupEvents()` already exists in `GlobalNotifications.tsx`. Extract it or pass grouped results into the notification effect rather than raw `newEvents`.
