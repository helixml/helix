# Implementation Tasks

- [ ] Create `frontend/src/hooks/useNavigationHistory.ts` — subscribes to router5 route changes, stores `{ url, routeName, params, title, timestamp }` in `localStorage` (`helix_nav_history`), deduplicates by URL (most recent wins), caps at 30 entries, returns history array
- [ ] In `GlobalNotifications.tsx`, refactor the existing event grouping logic (lines 59-97) to deduplicate alerts by `spec_task_id`, keeping only the most recent event per task (fall back to `idempotency_key` when `spec_task_id` is null)
- [ ] In `GlobalNotifications.tsx`, call `useNavigationHistory()` and compute the filtered "recently visited" list (exclude pages already covered by active alerts, cap at 10)
- [ ] In `GlobalNotifications.tsx`, render the "Recently visited" section below the alerts list — section heading + clickable rows with title + `router.navigate(routeName, params)` on click; hide section entirely when list is empty
- [ ] Verify: visiting Task A then Task B then Task A again → Task A appears once (at top) in recently visited
- [ ] Verify: a task with an active alert does not also appear in the recently visited section
- [ ] Verify: history survives page refresh (localStorage)
- [ ] Verify: alert deduplication — a task with both `specs_pushed` and `agent_interaction_completed` shows only one entry in the alerts section
