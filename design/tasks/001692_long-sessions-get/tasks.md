# Implementation Tasks

- [ ] Add `useListInteractions(sessionId, page, perPage)` React Query hook to `frontend/src/services/sessionService.ts` calling `GET /api/v1/sessions/{id}/interactions`
- [ ] Add `PAGE_SIZE = 20` constant and `olderInteractions` state to `EmbeddedSessionView`
- [ ] On session load, show only `session.interactions.slice(-PAGE_SIZE)` as initial interactions
- [ ] Track `hasOlderInteractions` (true when `session.interactions.length > PAGE_SIZE` or older pages exist)
- [ ] Render "Load older messages" button at top of list when `hasOlderInteractions` is true
- [ ] Implement load-more handler: prepend fetched interactions to `olderInteractions`, preserve scroll position (save/restore `scrollHeight` delta)
- [ ] Verify sticky-scroll-to-bottom still works correctly during streaming with the sliced interaction list
- [ ] Test with a session that has 50+ interactions: confirm no jank on open and load-more works
