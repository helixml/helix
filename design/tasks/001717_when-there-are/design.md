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

## Codebase Patterns Found

- `frontend/src/index.tsx` — Entry point, already has `window.emitError()` global function
- `frontend/src/App.tsx` — Root component, no error boundary wrapping it
- `frontend/src/components/system/Snackbar.tsx` — Existing error notification (only for caught errors, not crashes)
- `frontend/src/hooks/useAnalyticsInit.ts` — Sentry setup, uses `window.emitErrorFunctions`
- `frontend/index.html` — Has `<div id="root">`, need to add `<div id="error-overlay">`
