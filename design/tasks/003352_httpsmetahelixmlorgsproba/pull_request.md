# Fix Arc iOS login blocked by opaque script errors

## Summary
Prevent the mobile error overlay from treating WebKit's opaque `Script error.` events as fatal application failures. Arc on iOS can emit these events for cross-origin or browser-injected scripts without a source or stack, which previously covered the login page with the error overlay.

The mobile handler suppresses the overlay only when `window.onerror` provides the complete opaque-event signature: an exact generic message, no source, zero coordinates, and no error object. Suppressed events still reach error telemetry, while rejected promises and errors with actionable provenance keep the overlay and session log.

## Testing
- `yarn test src/utils/mobileErrorNoise.test.ts` — passed (2 tests)
- `yarn tsc` — passed
- `yarn build` — passed
