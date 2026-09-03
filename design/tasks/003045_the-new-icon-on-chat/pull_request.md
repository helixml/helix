# Always show the new chat action

## Summary
Keep the “+ New” action in each chat project visible at all times so new users can discover how to start a chat without first hovering or scrolling. Remove the hover-only visibility styling and add regression coverage for the persistent affordance.

## Testing
- `yarn test src/components/session/ProjectChatGroup.test.tsx` — 15 tests passed.
- `yarn tsc` — passed.
