# Implementation Tasks

## Phase 1: Add Render-Limiting to EmbeddedSessionView
- [x] Add `INTERACTIONS_TO_RENDER = 20` constant to `EmbeddedSessionView`
- [x] Slice interactions to show only the most recent N
- [x] Add "Show older messages" button that expands the rendered slice
- [~] Fix scroll-to-bottom bug (port pattern from Session.tsx)
- [ ] Test: confirm long sessions render fast and scroll works

## Phase 2: Add Data-Level Pagination
- [ ] Update `session_interaction_handlers.go` to return `totalCount` in the API response (already computed but not returned)
- [ ] Add `useListInteractions(sessionId, page, perPage)` React Query hook to `sessionService.ts`
- [ ] Update `Session.tsx` to use paginated API: initially fetch only last 20 interactions, auto-load older on scroll-up
- [ ] Preserve scroll position when loading older interactions (save/restore `scrollHeight` delta)
- [ ] Test with a session that has 50+ interactions: confirm fast load, no jank, scroll-to-bottom reliable
