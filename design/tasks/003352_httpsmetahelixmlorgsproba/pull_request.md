# Fix Arc iOS login blocked by opaque script errors

## Summary
Prevent the mobile error overlay from treating WebKit's opaque `Script error.` events as fatal application failures. Arc on iOS can emit these events for cross-origin or browser-injected scripts without a source or stack, which previously covered the login page with the error overlay.

The shared mobile error filter now ignores only the exact opaque message while preserving actionable application errors.

## Testing
- `yarn test src/utils/mobileErrorNoise.test.ts` — passed (2 tests)
- `yarn tsc` — passed
- `yarn build` — passed
