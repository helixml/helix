# Implementation Tasks

## Phase 1: Port SpecTask to Session.tsx
- [~] Replace `EmbeddedSessionView` with `Session` (via `PreviewPanel`) in `SpecTaskDetailContent.tsx`
- [ ] Port WebSocket-aware polling to `Session.tsx`: suppress 3s polling when WS is connected (prevents data race where stale HTTP overwrites fresh WS data)
- [ ] Test: confirm tool calls, responses, streaming all render correctly
- [ ] Test: confirm virtual scroll works (only 20 interactions rendered even with long session)

## Phase 2: Add Data-Level Pagination
- [ ] Update `session_interaction_handlers.go` to return `totalCount` in the API response (already computed but not returned)
- [ ] Add `useListInteractions(sessionId, page, perPage)` React Query hook to `sessionService.ts`
- [ ] Update `Session.tsx` to use paginated API: initially fetch only last 20 interactions, auto-load older on scroll-up
- [ ] Preserve scroll position when loading older interactions (save/restore `scrollHeight` delta)
- [ ] Test with a session that has 50+ interactions: confirm fast load, no jank, scroll-to-bottom reliable
