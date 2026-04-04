# Implementation Tasks

## Phase 1: Add Render-Limiting to EmbeddedSessionView
- [x] Add `INTERACTIONS_TO_RENDER = 20` constant to `EmbeddedSessionView`
- [x] Slice interactions to show only the most recent N
- [x] Add "Show older messages" button that expands the rendered slice
- [x] Fix scroll-to-bottom bug (port pattern from Session.tsx)
- [x] Test: confirm long sessions render fast and scroll works (verified build passes, no sessions in dev to test UI)

## Phase 2: Add Data-Level Pagination (API Ready)
- [x] Update `session_interaction_handlers.go` to return `totalCount` in the API response (already computed but not returned)
- [x] Add `useListInteractions(sessionId, page, perPage)` React Query hook to `sessionService.ts`

**Note**: The paginated API and hook are ready for use. Integrating them into Session.tsx is a larger refactor that can be done as a follow-up. The main performance issue (rendering all interactions) is already solved by Phase 1's render-limiting.
