# Design: Mobile Error Debug Overlay

## Architecture

Two complementary error-catching mechanisms, both gated to mobile/tablet only:

### 1. React ErrorBoundary (catches React render errors)

A class component `ErrorBoundary` wrapping `<App />` in `index.tsx`. On mobile, it renders an error overlay. On desktop, it re-throws so errors propagate normally.

**Location:** `frontend/src/components/system/ErrorBoundary.tsx`

```
index.tsx
  └── <ErrorBoundary>     ← NEW (class component, required for componentDidCatch)
        └── <App />
```

### 2. Global error handlers (catches non-React JS errors)

`window.onerror` and `window.addEventListener('unhandledrejection', ...)` registered in `index.tsx`, before React mounts. On mobile, these render a plain HTML error overlay into a dedicated `<div id="error-overlay">` in `index.html`. On desktop, they do nothing (let errors propagate).

**Why plain HTML, not React?** If React itself has crashed, we can't rely on React to render the error. The global handlers must work even when the React tree is completely dead.

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

Minimal inline-styled HTML (no CSS dependencies):
- Red border at top
- Error message in monospace
- Stack trace in a scrollable `<pre>` block
- Timestamp
- "Copy Error" button (uses `navigator.clipboard.writeText`)
- "Reload Page" button (calls `location.reload()`)
- Dark background with white text for readability

### Sentry Integration

Both the ErrorBoundary and global handlers call `window.emitError(error)` before displaying the overlay. This feeds into the existing Sentry pipeline in `useAnalyticsInit.ts`. No changes needed to the Sentry setup.

## Key Decisions

1. **Mobile-only overlay, desktop passes through** — The user explicitly wants desktop errors to propagate normally. Desktop browsers handle errors more gracefully and have dev tools.

2. **Works in prod builds** — No conditional compilation or dev-only gating. The overlay is always available on mobile.

3. **Plain HTML for global handler overlay** — React may be dead when these fire. The ErrorBoundary uses React for its overlay since React is still functional at that point.

4. **Single file for ErrorBoundary** — Keeps it simple. The global handlers go directly in `index.tsx` since they need to register before React mounts.

## Codebase Patterns Found

- `frontend/src/index.tsx` — Entry point, already has `window.emitError()` global function
- `frontend/src/App.tsx` — Root component, no error boundary wrapping it
- `frontend/src/components/system/Snackbar.tsx` — Existing error notification (only for caught errors, not crashes)
- `frontend/src/hooks/useAnalyticsInit.ts` — Sentry setup, uses `window.emitErrorFunctions`
- `frontend/index.html` — Has `<div id="root">`, need to add `<div id="error-overlay">`
