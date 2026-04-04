# Fix long sessions getting unusably slow in SpecTask chat

## Summary
Long-running spec task sessions with 50+ interactions were causing the chat UI to become sluggish. This PR adds render-limiting to only display the last 20 interactions, with a button to load older messages. Also improves scroll-to-bottom reliability.

## Changes
- Add `INTERACTIONS_TO_RENDER = 20` constant to `EmbeddedSessionView`
- Slice interactions to only render the most recent N
- Add "Show older messages" button that expands the rendered slice
- Fix scroll-to-bottom: add effect to scroll when streaming ends
- Update `/api/v1/sessions/{id}/interactions` to return `PaginatedInteractions` with `totalCount`
- Add `useListInteractions` React Query hook for paginated interaction fetching

## Testing
- Build passes (frontend and API)
- No sessions in dev environment to test UI, but the implementation follows the same pattern as `Session.tsx`

## Notes
- Kept `EmbeddedSessionView` instead of switching to `Session.tsx` because Session.tsx reads session ID from URL params, making it unsuitable for embedded use
- The paginated API hook is ready for future integration into Session.tsx if needed
