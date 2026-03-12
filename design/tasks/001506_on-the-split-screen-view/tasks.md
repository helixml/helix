# Implementation Tasks

- [ ] Add `embedded?: boolean` prop to `SpecTaskDetailContentProps` interface in `SpecTaskDetailContent.tsx`
- [ ] Pass `embedded` prop through to the component body and default it to `false`
- [ ] Guard `getInitialView` — when `embedded` is true, return `"desktop"` (or `"details"` if no session) instead of reading `router.params.view`
- [ ] Guard the `useEffect` that syncs `currentView` from `router.params.view` (line ~287) — skip entirely when `embedded` is true
- [ ] Guard `handleViewChange` — when `embedded` is true, only call `setCurrentView(newView)` without calling `router.mergeParams`
- [ ] Guard the auto-switch `useEffect` (line ~503) — when `embedded` is true, still call `setCurrentView` but skip `router.mergeParams`
- [ ] Guard the `streaming.setCurrentSessionId()` call (line ~493) — skip when `embedded` is true to avoid panels fighting over the global session
- [ ] Pass `embedded={true}` to `SpecTaskDetailContent` in `TabsView.tsx` `TaskPanel` (line ~1461)
- [ ] Verify standalone `SpecTaskDetailPage` does NOT pass `embedded` (keeps existing URL-synced behavior)
- [ ] Build frontend (`cd frontend && yarn build`) and verify no build errors
- [ ] Manual test: open two tasks in split screen, toggle Desktop/Chat/Details in one panel — confirm the other panel does NOT change
- [ ] Manual test: click "Start Planning" in one panel — confirm only that panel switches to desktop view
- [ ] Manual test: verify standalone task detail page (`/projects/:id/tasks/:taskId?view=desktop`) still syncs view with URL