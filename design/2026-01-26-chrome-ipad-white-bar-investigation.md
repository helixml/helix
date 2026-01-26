# Chrome iPad White Bar at Bottom - Investigation

**Date**: 2026-01-26
**Status**: Abandoned (too complex, needs deeper architectural changes)
**Affects**: Chrome on iPad (landscape mode, with Magic Keyboard, and portrait mode)

## Problem Description

On Chrome on iPad, a white bar appears at the bottom of the screen in the area where the iOS "Home Indicator" lives (the horizontal bar you swipe up to go home). This does NOT happen on Safari - only Chrome.

The issue occurs:
- On page load (not from scrolling)
- In landscape mode with Magic Keyboard attached
- In portrait mode after detaching keyboard and reloading
- When Chrome "thinks" the page is scrolled to the bottom

## Root Cause

### WebKit Bug 297779
- https://bugs.webkit.org/show_bug.cgi?id=297779
- Viewport and layout coordinates become desynchronized during initialization, keyboard interaction, or orientation changes
- Fixed interface elements shift upward, creating a gap between content and screen edge
- This gap exposes the `WKWebView` backing store (which is white by default)

### Why Chrome but not Safari?
Chrome on iOS uses WebKit (Apple requirement), but Chrome has additional layers and different initialization timing. The bug manifests differently in Chrome's WKWebView implementation.

### Key Technical Issue
The Helix frontend uses a **fixed-height container layout**:
- `html, body, #root` have `height: 100%` and `overflow: hidden`
- Layout.tsx uses `height: 100%` with `overflow: hidden` on all containers
- Scrolling happens INSIDE components (internal scroll containers)
- Chrome interprets this as "the page is at the scroll bottom" because there's no body scroll

## What We Tried (All Failed)

### CSS Approaches
1. `viewport-fit=cover` meta tag - Already had this
2. `theme-color` meta tag with dark color - No effect on Chrome
3. `color-scheme: dark` meta tag - No effect
4. `apple-mobile-web-app-status-bar-style: black-translucent` - No effect
5. `min-height: 100dvh` / `100vh` / `-webkit-fill-available` - Various combinations, none worked
6. `transform: translateZ(0)` on #root - Solution from df.tips article, didn't work
7. `env(safe-area-inset-bottom)` padding - No effect
8. `body::before` pseudo-element extending below viewport - No effect
9. Invisible div with `height: 100vh; position: absolute` (Quasar workaround) - No effect
10. `position: fixed` on #root - Made things worse

### JavaScript Approaches
1. `window.scrollTo(0, 1)` on load - No effect (issue happens on load)
2. Reset layout on `focusout`/`resize`/`orientationchange` events - No effect

## Proposed Solution (Not Implemented)

The user suggested a fundamental architectural change:

### Change from Internal Scroll to Page Scroll
Instead of:
```
html/body (#root) [overflow: hidden, height: 100%]
  └── Layout [overflow: hidden]
        ├── Sidebar [fixed height]
        └── Main Content [internal scroll]
```

Change to:
```
html/body [natural scroll, no overflow: hidden]
  └── Layout [no fixed height]
        ├── Sidebar [position: sticky, top: 0]
        └── Main Content [flows naturally, determines page height]
```

This would make Chrome NOT think the page is "at the bottom" because:
1. The page would actually be scrollable via native browser scroll
2. Content determines the page height naturally
3. Chrome's viewport calculations would work correctly

### Why We Abandoned This
When we started implementing this change:
1. Removed `overflow: hidden` from html/body/#root
2. Changed `height: 100%` to `minHeight: 100%` in Layout
3. Scrolling broke completely on the Projects page

The issue is that **many components depend on the current fixed-height container model**:
- The Drawer uses `position: relative` on desktop with `height: 100%`
- Internal scroll containers expect their parent to have a fixed height
- The entire layout system assumes `overflow: hidden` on parents

### What Would Be Needed
A complete audit and refactor of:
1. `Layout.tsx` - Make sidebar `position: sticky` instead of permanent drawer
2. `Page.tsx` - Remove fixed height constraints
3. `Sidebar.tsx` - Make it work with sticky positioning
4. All pages that use internal scroll - Ensure they work with page scroll
5. Mobile drawer behavior - Needs to remain `position: fixed` for temporary drawer

## References

- [WebKit Bug 297779](https://bugs.webkit.org/show_bug.cgi?id=297779) - Root cause
- [df.tips article](https://df.tips/t/topic/2702) - Detailed analysis of the bug
- [VS Code Issue #149048](https://github.com/microsoft/vscode/issues/149048) - Same issue in VS Code on iPad
- [Quasar Issue #6695](https://github.com/quasarframework/quasar/issues/6695) - Similar issue with workarounds

## Files Modified (All Reverted)

- `frontend/index.html` - Various CSS attempts
- `frontend/src/pages/Layout.tsx` - Layout changes
- `frontend/src/components/system/Page.tsx` - Height changes

## Current State

All changes have been reverted. The white bar issue remains on Chrome on iPad.

## Future Work

If this issue becomes a priority:
1. Consider a major refactor to page-scroll architecture
2. Make the sidebar use `position: sticky` on desktop
3. Audit all components that use internal scrolling
4. Test thoroughly on iPad Chrome in all orientations
5. Consider if this is even fixable given it's a WebKit bug that Apple hasn't fixed
