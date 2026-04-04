# Implementation Tasks

- [ ] Add `useListInteractions(sessionId, page, perPage)` React Query hook to `sessionService.ts` calling `GET /api/v1/sessions/{id}/interactions`
- [ ] Update `Session.tsx` to use paginated API: initially fetch only last 20 interactions, auto-load older on scroll-up
- [ ] Replace `EmbeddedSessionView` with `Session` (via `PreviewPanel`) in `SpecTaskDetailContent.tsx`
- [ ] Preserve scroll position when loading older interactions (save/restore `scrollHeight` delta)
- [ ] Test with a session that has 50+ interactions: confirm no jank on open, virtual scroll works, scroll-to-bottom is reliable
