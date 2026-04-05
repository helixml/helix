# Requirements: Mobile Error Debug Overlay

## Problem

When JavaScript errors occur on iOS/Safari, the page crashes. Safari shows **"A problem repeatedly occurred"** with page title **"Web Page Crashed"**. This is a WebKit content process crash — it happens when either (a) unhandled JS errors cause repeated page failures triggering Safari's crash detection, or (b) GPU/memory exhaustion causes iOS Jetsam to kill the WebKit process. Either way, the user sees a dead page with no debug info — especially problematic on iPad where dev tools aren't available.

The prod build at meta.helix.ml is heavily tested on iPad, so this must work in production builds too.

## User Stories

1. **As a developer testing on iPad**, when a JS error crashes the page, I want to see the error message and stack trace so I can debug without desktop dev tools.

2. **As a developer testing on iPad**, when the page crashes and Safari reloads it, I want to see what errors happened *before* the crash so I'm not completely in the dark.

3. **As a desktop user**, I want errors to propagate normally (they rarely crash the full page on desktop, and I have dev tools available).

## Acceptance Criteria

- [ ] When an unhandled JS error or React render error occurs on a mobile/tablet device, a styled error overlay is displayed instead of a white screen
- [ ] The overlay shows: error message, stack trace (if available), and a timestamp
- [ ] The overlay includes a "Copy to clipboard" button so the user can share the error info
- [ ] The overlay includes a "Reload" button to recover
- [ ] Errors are persisted to `sessionStorage` so that after a WebKit process crash and reload, the previous errors are displayed
- [ ] On desktop browsers, errors propagate to the top level as they do today (no overlay)
- [ ] Works in both dev and production builds (no `process.env.NODE_ENV` gating)
- [ ] The error overlay catches both React component errors (via ErrorBoundary) and non-React errors (via `window.onerror` / `unhandledrejection`)
- [ ] Existing Sentry integration continues to work — errors are still reported to Sentry when configured
- [ ] The error overlay itself avoids GPU-heavy CSS properties (no `position: fixed`, no `filter`, no opacity animations) to prevent compounding the crash
