# Preserve scroll position while chat updates

## Summary
Keep the chat viewport at the reader's current position when new output arrives or streaming completes. Auto-scroll remains enabled while the viewport is near the latest message, and explicitly sending or regenerating a message still moves to the new turn.

## Testing
- `yarn test src/pages/Session.orgRestartBanner.test.tsx src/components/session/EmbeddedSessionView.test.tsx` — passed (5 tests).
- `yarn tsc` — passed.
