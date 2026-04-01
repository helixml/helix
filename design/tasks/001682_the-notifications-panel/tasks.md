# Implementation Tasks

- [x] In `api/pkg/store/store_attention_events.go`: wrap the `DISTINCT ON` query in a subquery and add `ORDER BY created_at DESC` on the outer query so results are returned newest-first
- [x] In `frontend/src/components/system/GlobalNotifications.tsx`: add a `groupTimestamp()` helper and sort groups newest-first after `deduplicateGroupsByTask()` (apply in both the render path at line 453 and the browser notification `useEffect` at line 357)
- [x] Fix the incorrect comment at line 107 of `GlobalNotifications.tsx` that claims the API returns events newest-first
- [~] Verify in the browser that the notifications panel now shows the most recent notification at the top after a new event arrives
