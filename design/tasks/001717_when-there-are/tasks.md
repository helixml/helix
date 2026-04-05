# Implementation Tasks

## Error Overlay (Path 1: JS error cascade prevention)

- [x] Add `isMobileOrTablet()` utility function (UA + `maxTouchPoints` check for iPad which reports as "Macintosh")
- [x] Create `ErrorBoundary` class component in `frontend/src/components/system/ErrorBoundary.tsx` — on mobile: renders error overlay with message, stack, copy & reload buttons; on desktop: re-throws the error
- [x] Add `<div id="error-overlay">` to `frontend/index.html` (sibling of `<div id="root">`)
- [x] Add `window.onerror` and `unhandledrejection` handlers in `frontend/src/index.tsx` — on mobile: render plain HTML error info into `#error-overlay`; on desktop: no-op (let errors propagate)
- [x] Add sessionStorage error logging — write errors as they occur, read on page load to show "errors before crash" after a WebKit process crash + reload
- [x] Wire both error paths to call `window.emitError()` so Sentry continues to receive reports
- [x] Wrap `<App />` with `<ErrorBoundary>` in `frontend/src/index.tsx`
- [x] Ensure error overlay uses GPU-safe CSS only (no `position: fixed`, no `filter`, no opacity animations, no `will-change`) — use `position: absolute` on body

## Streaming GPU/Memory Optimizations (Path 2: reduce Jetsam kills)

- [x] Cap canvas rendering resolution to device screen size on mobile/tablet — in `renderVideoFrame()` (`websocket-stream.ts:954-956`), cap canvas dimensions to `screen.width * devicePixelRatio` × `screen.height * devicePixelRatio` instead of matching decoded frame size. iPad Pro (2732×2048) keeps full res; smaller devices save GPU memory (e.g. 4K stream on iPad Air: ~22MB canvas vs ~33MB)
- [x] Change `opacity: 0` to `visibility: hidden` on canvas in screenshot mode (`DesktopStreamViewer.tsx:5093`) on mobile — `opacity: 0` still allocates GPU memory for compositing
- [x] Remove CSS `filter: drop-shadow(...)` from `AgentCursorOverlay.tsx:60` on mobile — set to `none` instead
- [x] Remove CSS `filter: glowFilter` from `CursorRenderer.tsx:366` on mobile — set to `none` instead
- [x] Remove `willChange: "transform"` from trackpad cursor div (`DesktopStreamViewer.tsx:5165`) on mobile — permanently allocates a GPU layer
- [x] Simplify `filter: grayscale(0.5) brightness(0.7) blur(1px)` on paused desktop (`ExternalAgentDesktopViewer.tsx:373`) on mobile — use simple `opacity: 0.4` instead of triple GPU filter

## Testing

- [ ] Test on iPad/Safari: trigger a JS error and verify the overlay appears instead of white screen / "A problem repeatedly occurred"
- [ ] Test on iPad/Safari: open a desktop streaming session and verify reduced GPU pressure (no crash at default resolution)
- [ ] Test on desktop: verify errors still propagate normally (no overlay, dev tools still work)
- [ ] Test on desktop: verify streaming visual quality is unchanged (filters/glow still present)
- [x] Verify prod build works: `cd frontend && yarn build` succeeds, overlay functional in built output
