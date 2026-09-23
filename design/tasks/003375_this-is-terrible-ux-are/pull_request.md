# Improve insufficient-credit errors in chat

## Summary
Improve failed chat states in regular and agent conversations by replacing internal error language with clear, user-facing guidance. Insufficient-credit errors now show one relevant action at a time: a prominent Add credits button below the server-configured minimum inference balance, or Retry once enough credits are available. Other failures use friendlier copy while retaining their diagnostic message.

## Testing
- `yarn test InteractionInference.errorDisplay.test.tsx` — 5 tests passed.
- `yarn tsc --pretty false` — passed.
- `go test ./pkg/controller ./pkg/types` — passed.
