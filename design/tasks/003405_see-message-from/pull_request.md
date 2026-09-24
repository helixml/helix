# Prevent the welcome screen from flashing while chats load

## Summary
Prevent the new-chat welcome screen from appearing while the interaction query is still loading. This avoids the initial-state flash seen when reloading a running chat or switching between bots.

## Testing
- `yarn test src/components/session/minimalChatLogic.test.ts` — 9 tests passed.
- `yarn tsc` — passed.
