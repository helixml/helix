# Implementation Tasks

- [ ] Add `isMobileOrTablet()` utility function (UA + `maxTouchPoints` check for iPad which reports as "Macintosh")
- [ ] Create `ErrorBoundary` class component in `frontend/src/components/system/ErrorBoundary.tsx` — on mobile: renders error overlay with message, stack, copy & reload buttons; on desktop: re-throws the error
- [ ] Add `<div id="error-overlay">` to `frontend/index.html` (sibling of `<div id="root">`)
- [ ] Add `window.onerror` and `unhandledrejection` handlers in `frontend/src/index.tsx` — on mobile: render plain HTML error info into `#error-overlay`; on desktop: no-op (let errors propagate)
- [ ] Add sessionStorage error logging — write errors as they occur, read on page load to show "errors before crash" after a WebKit process crash + reload
- [ ] Wire both error paths to call `window.emitError()` so Sentry continues to receive reports
- [ ] Wrap `<App />` with `<ErrorBoundary>` in `frontend/src/index.tsx`
- [ ] Ensure error overlay uses GPU-safe CSS only (no `position: fixed`, no `filter`, no opacity animations, no `will-change`) — use `position: absolute` on body
- [ ] Test on iPad/Safari: trigger a JS error and verify the overlay appears instead of white screen / "A problem repeatedly occurred"
- [ ] Test on desktop: verify errors still propagate normally (no overlay, dev tools still work)
- [ ] Verify prod build works: `cd frontend && yarn build` succeeds, overlay functional in built output
