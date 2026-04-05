# Implementation Tasks

- [ ] Add `isMobileOrTablet()` utility function (UA + touch detection for iPad)
- [ ] Create `ErrorBoundary` class component in `frontend/src/components/system/ErrorBoundary.tsx` — on mobile: renders error overlay with message, stack, copy & reload buttons; on desktop: re-throws the error
- [ ] Add `<div id="error-overlay">` to `frontend/index.html` (sibling of `<div id="root">`)
- [ ] Add `window.onerror` and `unhandledrejection` handlers in `frontend/src/index.tsx` — on mobile: render plain HTML error info into `#error-overlay`; on desktop: no-op (let errors propagate)
- [ ] Wire both error paths to call `window.emitError()` so Sentry continues to receive reports
- [ ] Wrap `<App />` with `<ErrorBoundary>` in `frontend/src/index.tsx`
- [ ] Test on iPad/Safari: trigger a JS error and verify the overlay appears instead of a white screen
- [ ] Test on desktop: verify errors still propagate normally (no overlay, dev tools still work)
- [ ] Verify prod build works: `cd frontend && yarn build` succeeds, overlay functional in built output
