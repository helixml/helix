# Design: Mobile Error Debug Overlay

## Research: Safari's "A problem repeatedly occurred" / "Web Page Crashed"

This is Safari's message when the **WebKit content process crashes**. On iOS, each tab runs in a separate WebKit process. When that process crashes (or is killed by iOS Jetsam for exceeding memory/GPU limits), Safari shows the "A problem repeatedly occurred on [URL]" page with title "Web Page Crashed".

**This is NOT a catchable JavaScript error.** `window.onerror`, `unhandledrejection`, and React ErrorBoundary cannot intercept a process-level crash. However, there are **two distinct failure paths** that lead to this message:

### Path 1: JS Error Cascade (ErrorBoundary can fix this)
1. A JavaScript error occurs (e.g. uncaught exception, React render error)
2. React unmounts → page goes white
3. Safari detects the page is broken, auto-reloads
4. Same JS error occurs again on reload
5. After repeated crashes, Safari gives up → **"A problem repeatedly occurred"**

**An ErrorBoundary breaks this loop** by catching the error and rendering debug info instead of a white page. Safari sees a functioning page (not a crash), so it doesn't trigger its reload/crash cycle.

### Path 2: GPU/Memory Exhaustion (needs different mitigations)
1. Page uses too much GPU memory (composited layers, large elements, animations)
2. iOS Jetsam kills the WebKit process for exceeding memory limits
3. Safari shows **"A problem repeatedly occurred"**

This path can't be caught by JavaScript at all — the process is already dead.

**Known GPU/memory triggers** (from StackOverflow, confirmed by WebKit source):
- `position: fixed/sticky` — creates composited layers
- CSS 3D transforms (`translateZ`, `translate3d`) — forces GPU compositing
- `filter` CSS property — GPU-composited
- `will-change` CSS property — pre-allocates GPU layers
- `-webkit-overflow-scrolling: touch` — creates composited scroll layers
- `backface-visibility: hidden` — GPU-composited
- Animating `transform` or `opacity` — triggers layer composition
- `iframe` elements — each one is a composited layer
- Large semi-transparent overlays (e.g. full-page modal backdrops)
- Content overflowing body on x-axis → GPU spikes during scroll
- CSS opacity transitions on dynamically loaded content

**Key insight from SO (milehighsi)**: "GPU on one of our pages was peaking at 550mb, and is now 12mb" after fixing compositing issues. Larger/higher-density displays (iPad Pro) are hit harder.

### Sources
- [StackOverflow: Debugging iOS Safari crash "A problem repeatedly occurred"](https://stackoverflow.com/questions/76127296/debugging-a-ios-safari-crash-a-problem-repeatedly-occurred) — detailed answer from milehighsi about GPU compositing causes
- [xjavascript.com: How to Debug iOS Safari crash](https://www.xjavascript.com/blog/debugging-a-ios-safari-crash-a-problem-repeatedly-occurred/) — confirms WebKit rendering/performance bottleneck, large DOM trees (>10k elements)
- [Bubble Forum: Safari on iOS Crashes Repeatedly](https://forum.bubble.io/t/safari-on-ios-crashes-repeatedly-a-problem-repeatedly-occurred-error/353936) — confirms Chrome mobile also affected but less often

## Architecture

Two-pronged approach: **catch JS errors** (Path 1) and **persist error logs** so they survive the crash/reload cycle (both paths).

### 1. React ErrorBoundary (catches React render errors — Path 1)

A class component `ErrorBoundary` wrapping `<App />` in `index.tsx`. On mobile, it renders an error overlay. On desktop, it re-throws so errors propagate normally.

**Location:** `frontend/src/components/system/ErrorBoundary.tsx`

```
index.tsx
  └── <ErrorBoundary>     ← NEW (class component, required for componentDidCatch)
        └── <App />
```

### 2. Global error handlers (catches non-React JS errors — Path 1)

`window.onerror` and `window.addEventListener('unhandledrejection', ...)` registered in `index.tsx`, before React mounts. On mobile, these render a plain HTML error overlay into a dedicated `<div id="error-overlay">` in `index.html`. On desktop, they do nothing (let errors propagate).

**Why plain HTML, not React?** If React itself has crashed, we can't rely on React to render the error. The global handlers must work even when the React tree is completely dead.

### 3. Error log persistence via sessionStorage (helps debug Path 2)

Since Path 2 crashes kill the process entirely, we can't show an overlay at crash time. But we can:
- Log every JS error to `sessionStorage` as it occurs (before it potentially cascades into a crash)
- On page load, check `sessionStorage` for recent errors from a previous session
- If found, display them in the overlay — "These errors occurred before the page crashed"

This gives the user visibility into what happened even after a Jetsam kill, because `sessionStorage` survives page reloads within the same tab.

### Mobile Detection

Simple UA-based check — not perfect, but good enough for this use case:

```typescript
function isMobileOrTablet(): boolean {
  return /iPhone|iPad|iPod|Android/i.test(navigator.userAgent) ||
    (navigator.maxTouchPoints > 1 && !window.matchMedia('(pointer: fine)').matches)
}
```

The iPad UA check is important — modern iPads report as "Macintosh" in the UA string, so we also check `maxTouchPoints > 1` combined with no fine pointer (mouse).

### Error Overlay UI

Minimal inline-styled HTML (no CSS dependencies, no GPU-heavy properties):
- Red border at top
- Error message in monospace
- Stack trace in a scrollable `<pre>` block
- Timestamp
- Previous errors from `sessionStorage` (if page was reloaded after a crash)
- "Copy Error" button (uses `navigator.clipboard.writeText`)
- "Reload Page" button (calls `location.reload()`)
- Dark background with white text for readability
- **No CSS animations, no opacity transitions, no position:fixed** — the overlay itself must not trigger GPU compositing issues

### Sentry Integration

Both the ErrorBoundary and global handlers call `window.emitError(error)` before displaying the overlay. This feeds into the existing Sentry pipeline in `useAnalyticsInit.ts`. No changes needed to the Sentry setup.

## Key Decisions

1. **Mobile-only overlay, desktop passes through** — The user explicitly wants desktop errors to propagate normally. Desktop browsers handle errors more gracefully and have dev tools.

2. **Works in prod builds** — No conditional compilation or dev-only gating. The overlay is always available on mobile.

3. **Plain HTML for global handler overlay** — React may be dead when these fire. The ErrorBoundary uses React for its overlay since React is still functional at that point (but also uses minimal styling to avoid GPU issues).

4. **sessionStorage for error persistence** — Survives reloads within a tab, providing post-crash debug info. Using sessionStorage (not localStorage) so it's scoped to the tab and auto-cleans up.

5. **GPU-safe overlay styling** — The error overlay itself avoids all known GPU compositing triggers (`position: fixed`, `filter`, `will-change`, opacity animations). Uses `position: absolute` on the body with `overflow: auto`.

## Desktop Streaming GPU/Memory Analysis

Investigated the desktop streaming code (`DesktopStreamViewer.tsx`, `websocket-stream.ts`) for GPU/memory issues that could trigger the WebKit Jetsam kill on iPad, especially at 4K.

### Baseline Memory: 234MB Before Streaming Even Starts

Heap snapshot taken at the idle org picker page (no streaming session active, Chrome DevTools `performance.memory`):

| Category | Size |
|----------|------|
| External strings (JS source text retained by V8) | 66.8 MB |
| Compiled JS (bytecode) | 58.5 MB |
| Arrays | 27.1 MB |
| Other (hidden, shapes, maps) | 26.8 MB |
| JS objects | 19.1 MB |
| Closures | 14.2 MB |
| Source maps (inline base64) | 12.3 MB |
| JS strings (other) | 8.8 MB |
| **Total** | **233.7 MB** |

**Key concern:** 125MB is JS code (source text + bytecode) loaded eagerly. The entire app loads upfront — every page's code is in memory even if you're just looking at the org picker. The largest source maps retained in memory: `api.ts` (657KB), `DesktopStreamViewer.tsx` (434KB), `websocket-stream.ts` (233KB), plus 6.4MB of other component source maps.

**iOS Safari memory limits** are ~1-1.5GB per tab (varies by device, much lower on older iPads). Starting at 234MB baseline, then adding a streaming session (canvas GPU buffers + VideoDecoder + WebSocket buffers) means you're eating a significant chunk of the budget before any actual work happens.

**Note:** This measurement is from Chrome's V8 heap — Safari's JavaScriptCore will have different numbers, but the relative proportions (code dominating) should be similar. GPU memory (canvas backing stores, composited layers) is *additional* and not reflected in the JS heap at all.

**Follow-up opportunity (not in this task):** Route-based code splitting would reduce baseline memory by only loading code for the current page. The streaming code (DesktopStreamViewer, websocket-stream) is ~700KB of source maps alone and shouldn't be in memory until the user opens a streaming session.

### Canvas Memory at High Resolutions

The streaming canvas is dynamically sized to match the remote desktop resolution:
- **1080p**: 1920×1080×4 = ~8MB GPU memory (fine)
- **4K**: 3840×2160×4 = **~33MB GPU memory** (risky on iPad)
- **5K**: 5120×2880×4 = **~59MB GPU memory** (very risky)

The canvas uses `desynchronized: true` (low-latency mode), which creates its own composited layer. This is correct for performance, but it's a large GPU allocation that stacks with everything else on the page.

**The canvas resolution is set to match decoded frames** (`websocket-stream.ts:954-956` — canvas resizes to match `frame.displayWidth`/`frame.displayHeight`). On iPad, a 4K stream means a 4K canvas even though the iPad display can't show all those pixels when the stream is fitted to screen. This is the single biggest memory optimization opportunity. Note: we are NOT changing the server's stream resolution — just capping the canvas rendering size on the client side and letting `drawImage` downscale.

### GPU-Heavy Overlays Stacking on the Canvas

Multiple composited layers stack on top of the canvas during streaming:

| Element | GPU Trigger | File:Line |
|---------|-------------|-----------|
| Main canvas | `position: absolute` + `transform: translate(...)` + `desynchronized` | `DesktopStreamViewer.tsx:5080-5084` |
| Screenshot overlay `<img>` | `position: absolute` + `transform: translate(...)` | `DesktopStreamViewer.tsx:5127-5131` |
| Trackpad cursor div | **`willChange: "transform"`** (explicit GPU layer) | `DesktopStreamViewer.tsx:5165` |
| Agent cursor | **`filter: drop-shadow(0 0 6px ...) drop-shadow(0 0 12px ...)`** (double CSS filter!) | `AgentCursorOverlay.tsx:60` |
| Cursor glow | **`filter: glowFilter`** (another CSS filter) | `CursorRenderer.tsx:394,430` |
| Paused desktop img | **`filter: grayscale(0.5) brightness(0.7) blur(1px)`** + `opacity: 0.6` (triple filter!) | `ExternalAgentDesktopViewer.tsx:373` |
| Clipboard toast | `opacity` + `transform` transition animation | `DesktopStreamViewer.tsx:5280-5281` |
| Canvas in screenshot mode | `opacity: 0` (still composited — hidden but allocated) | `DesktopStreamViewer.tsx:5093` |

Each of these creates a separate GPU-composited layer. On iPad at 4K, that's the 33MB canvas PLUS multiple filter layers PLUS transform layers — all competing for WebKit's GPU memory budget.

### Specific Optimizations

**High impact (do in this task):**

1. **Cap canvas rendering resolution to device screen size on mobile/tablet** — When `isMobileOrTablet()`, cap the canvas dimensions to `window.screen.width * devicePixelRatio` × `window.screen.height * devicePixelRatio` in `renderVideoFrame()` instead of blindly matching the decoded frame dimensions. The server still sends the same stream — we just render into a canvas that matches the device's actual display capability, and `drawImage(frame, 0, 0, cappedWidth, cappedHeight)` downscales the frame. This means an iPad Pro (2732×2048 native) still gets its full resolution, while a smaller device doesn't waste GPU memory on pixels it can never display. A 4K stream (3840×2160) on an iPad Air 11" (2360×1640) would render at 2360×1640 instead, saving ~12MB of GPU memory with no visual quality loss since those extra pixels were being CSS-downscaled anyway.

2. **Use `display: none` instead of `opacity: 0`** — `DesktopStreamViewer.tsx:5093` sets `opacity: 0` on the canvas in screenshot mode. An element with `opacity: 0` is still composited (GPU memory allocated). Switch to `visibility: hidden` or `display: none` when not in video mode. Same for any other hidden-but-composited elements.

3. **Remove CSS `filter` on cursor overlays on mobile** — The `drop-shadow` filter on `AgentCursorOverlay.tsx:60` and the `glowFilter` on `CursorRenderer.tsx:394,430` each create GPU-composited layers. On mobile, replace these with simpler solid-color styling or remove the glow entirely.

4. **Remove `willChange: "transform"` from trackpad cursor** — `DesktopStreamViewer.tsx:5165` uses `willChange: "transform"` which permanently allocates a GPU layer. On iPad, this is unnecessary overhead; the cursor is small and updates are infrequent enough that regular compositing is fine. Remove on mobile.

**Medium impact (consider for follow-up):**

5. **Conditionally render overlays** — Remote cursor overlays, agent cursor overlays, stats panels, and connection overlays should use conditional rendering (`{condition && <Component />}`) rather than rendering with `display: none`. Unrendered components use zero GPU memory.

6. **Simplify paused desktop filter** — `ExternalAgentDesktopViewer.tsx:373` applies `grayscale(0.5) brightness(0.7) blur(1px)` — three GPU filters stacked. On mobile, use a simple dark overlay or CSS `opacity` alone (no blur/grayscale).

### Runtime Memory Monitoring

**Can we monitor our own memory and take evasive action?**

No direct API exists on Safari/iOS:
- `performance.memory` — Chrome-only, deprecated, never supported on Safari
- `performance.measureUserAgentSpecificMemory()` — Chrome 89+ only, not supported on Safari (tested through v26.5), also requires cross-origin isolation headers which would break integrations

**Proxy signals we can monitor instead** (we already track most of these in streaming stats):
- **VideoDecoder `decodeQueueSize`** — if growing, frames are backing up in memory
- **Frames dropped vs decoded ratio** — rising drop rate = device struggling
- **FPS drops** — sustained FPS well below target = resource pressure
- **Canvas pixel count** — we know exactly how much GPU memory the canvas uses (width × height × 4 bytes)

**Evasive actions when proxy signals indicate pressure (follow-up task):**
- Automatically downscale canvas resolution (e.g. drop from screen-native to 1080p)
- Flush VideoDecoder queue and request a fresh keyframe
- Log a warning to sessionStorage (so post-crash overlay can show "memory pressure detected before crash")

## Codebase Patterns Found

- `frontend/src/index.tsx` — Entry point, already has `window.emitError()` global function
- `frontend/src/App.tsx` — Root component, no error boundary wrapping it
- `frontend/src/components/system/Snackbar.tsx` — Existing error notification (only for caught errors, not crashes)
- `frontend/src/hooks/useAnalyticsInit.ts` — Sentry setup, uses `window.emitErrorFunctions`
- `frontend/index.html` — Has `<div id="root">`, need to add `<div id="error-overlay">`
- `frontend/src/lib/helix-stream/stream/websocket-stream.ts` — Main streaming engine, canvas resize at lines 954-956, decoder init at 863-936
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` — ~5300-line streaming UI component, canvas element at line 5051, overlay stack from line 5080
- `frontend/src/components/external-agent/AgentCursorOverlay.tsx` — Agent cursor with double drop-shadow filter at line 60
- `frontend/src/components/external-agent/CursorRenderer.tsx` — Cursor glow filter at lines 394, 430
- `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx` — Paused desktop with triple CSS filter at line 373
