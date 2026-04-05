# Requirements: Mobile Error Debug Overlay

## Problem

When JavaScript errors occur on iOS/Safari/Chrome mobile, the entire page crashes to a white screen (Safari shows "this page has had multiple errors"). There's no way to see what went wrong — especially problematic on iPad where dev tools aren't available. The prod build at meta.helix.ml is heavily tested on iPad, so this must work in production builds too.

## User Stories

1. **As a developer testing on iPad**, when a JS error crashes the page, I want to see the error message, stack trace, and component tree so I can debug without desktop dev tools.

2. **As a desktop user**, I want errors to propagate normally (they rarely crash the full page on desktop, and I have dev tools available).

## Acceptance Criteria

- [ ] When an unhandled JS error or React render error occurs on a mobile/tablet device, a styled error overlay is displayed instead of a white screen
- [ ] The overlay shows: error message, stack trace (if available), and a timestamp
- [ ] The overlay includes a "Copy to clipboard" button so the user can share the error info
- [ ] The overlay includes a "Reload" button to recover
- [ ] On desktop browsers, errors propagate to the top level as they do today (no overlay)
- [ ] Works in both dev and production builds (no `process.env.NODE_ENV` gating)
- [ ] The error overlay catches both React component errors (via ErrorBoundary) and non-React errors (via `window.onerror` / `unhandledrejection`)
- [ ] Existing Sentry integration continues to work — errors are still reported to Sentry when configured
