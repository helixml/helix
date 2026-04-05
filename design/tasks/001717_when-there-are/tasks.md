# Implementation Tasks

## Error Overlay (Path 1: JS error cascade prevention)

- [ ] Add `isMobileOrTablet()` utility function (UA + `maxTouchPoints` check for iPad which reports as "Macintosh")
- [ ] Create `ErrorBoundary` class component in `frontend/src/components/system/ErrorBoundary.tsx` — on mobile: renders error overlay with message, stack, copy & reload buttons; on desktop: re-throws the error
- [ ] Add `<div id="error-overlay">` to `frontend/index.html` (sibling of `<div id="root">`)
- [ ] Add `window.onerror` and `unhandledrejection` handlers in `frontend/src/index.tsx` — on mobile: render plain HTML error info into `#error-overlay`; on desktop: no-op (let errors propagate)
- [ ] Add sessionStorage error logging — write errors as they occur, read on page load to show "errors before crash" after a WebKit process crash + reload
- [ ] Wire both error paths to call `window.emitError()` so Sentry continues to receive reports
- [ ] Wrap `<App />` with `<ErrorBoundary>` in `frontend/src/index.tsx`
- [ ] Ensure error overlay uses GPU-safe CSS only (no `position: fixed`, no `filter`, no opacity animations, no `will-change`) — use `position: absolute` on body

## Streaming GPU/Memory Optimizations (Path 2: reduce Jetsam kills)

- [ ] Cap canvas resolution to 1080p on mobile/tablet — either send resolution cap in StreamInit or downscale canvas and let `drawImage` handle it. 4K canvas = ~33MB GPU vs 1080p = ~8MB
- [ ] Change `opacity: 0` to `display: none` or `visibility: hidden` on canvas in screenshot mode (`DesktopStreamViewer.tsx:5093`) — `opacity: 0` still allocates GPU memory for compositing
- [ ] Remove CSS `filter: drop-shadow(...)` from `AgentCursorOverlay.tsx:60` on mobile — use simpler solid-color styling instead
- [ ] Remove CSS `filter: glowFilter` from `CursorRenderer.tsx:394,430` on mobile — each filter creates a GPU-composited layer
- [ ] Remove `willChange: "transform"` from trackpad cursor div (`DesktopStreamViewer.tsx:5165`) on mobile — permanently allocates a GPU layer
- [ ] Simplify `filter: grayscale(0.5) brightness(0.7) blur(1px)` on paused desktop (`ExternalAgentDesktopViewer.tsx:373`) on mobile — use simple `opacity` or dark overlay instead of triple GPU filter

## Testing

- [ ] Test on iPad/Safari: trigger a JS error and verify the overlay appears instead of white screen / "A problem repeatedly occurred"
- [ ] Test on iPad/Safari: open a desktop streaming session and verify reduced GPU pressure (no crash at default resolution)
- [ ] Test on desktop: verify errors still propagate normally (no overlay, dev tools still work)
- [ ] Test on desktop: verify streaming visual quality is unchanged (filters/glow still present)
- [ ] Verify prod build works: `cd frontend && yarn build` succeeds, overlay functional in built output
