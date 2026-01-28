# Requirements: Chrome Swipe Navigation Fix

## Problem Statement

Chrome's native swipe-left/swipe-right gestures for browser back/forward navigation are broken in the Helix frontend. At the same time, any fix must not break Safari on iPad where previous attempts caused the UI to slide around.

## User Stories

1. **As a Chrome desktop user**, I want to swipe left/right (with two fingers on trackpad or via touch) to navigate browser history, so I can quickly go back and forward between pages.

2. **As an iPad Safari user**, I want the UI to remain stable when I interact with the app, without the entire interface sliding around unexpectedly.

3. **As a desktop stream viewer user**, I want touch/trackpad gestures on the video stream canvas to control the remote desktop, not trigger browser navigation.

## Root Cause Analysis

The `DesktopStreamViewer.tsx` component calls `event.preventDefault()` on all touch events (`handleTouchStart`, `handleTouchMove`, `handleTouchEnd`). This blocks Chrome's native swipe-to-navigate gesture.

Additionally, `index.html` has global `overscroll-behavior: none` which may contribute to blocking native browser gestures outside the stream viewer.

## Acceptance Criteria

1. **Chrome swipe navigation works** - On pages WITHOUT the stream viewer (e.g., project list, settings, task list), Chrome's two-finger swipe gestures trigger browser back/forward.

2. **Stream viewer captures gestures** - Inside the `DesktopStreamViewer` canvas, touch gestures are captured and sent to the remote desktop (current behavior preserved).

3. **Safari iPad stability** - Safari on iPad does not exhibit UI sliding/bouncing when interacting with the app.

4. **No regression in stream touch controls** - Trackpad mode (cursor movement, tap-to-click, pinch-to-zoom) continues working on the stream viewer.

## Constraints

- Must not introduce device-specific hacks if avoidable
- Should use standard CSS/JS APIs (`touch-action`, `overscroll-behavior`)
- Preserve existing touch mode functionality in stream viewer