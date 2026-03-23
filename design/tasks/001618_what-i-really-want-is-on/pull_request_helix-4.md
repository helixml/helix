# Add navigation history to notification panel

## Summary

Enhances the notification panel (bell icon, top-right) with two improvements:

1. **Alert deduplication by task** — when multiple attention events exist for the same spec task, only the most recent one is shown in the "Needs Attention" list.

2. **"Recently visited" section** — below the alerts, shows a list of spec task detail and review pages the user has visited recently (from localStorage), excluding any already shown as active alerts. Clicking an entry navigates back to that page. Hidden when empty.

## Changes

- `frontend/src/lib/navHistory.ts` — new module: pure localStorage read/write for navigation history, no circular imports
- `frontend/src/router.tsx` — import `navHistory` and call `recordNavRoute` in the existing `router.subscribe()` callback so all navigations are recorded globally
- `frontend/src/hooks/useNavigationHistory.ts` — new hook: reads history from localStorage, re-renders on route change via `useRoute()`
- `frontend/src/components/system/GlobalNotifications.tsx` — add `deduplicateGroupsByTask()`, `RecentPageItem` component, and the "Recently visited" section beneath the alerts list
