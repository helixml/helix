# Design: Chrome Swipe Navigation Fix

## Overview

Fix Chrome's native swipe-to-navigate gestures while preserving Safari iPad stability and stream viewer touch controls.

## Technical Analysis

### Current Behavior

1. **Global CSS** (`index.html`):
   ```css
   html, body, #root {
     overscroll-behavior: none;
   }
   ```
   This prevents pull-to-refresh and bounce effects but may also affect swipe navigation.

2. **DesktopStreamViewer** (`DesktopStreamViewer.tsx`):
   - `handleTouchStart`: calls `event.preventDefault()` unconditionally
   - `handleTouchMove`: calls `event.preventDefault()` unconditionally
   - `handleTouchEnd`: calls `event.preventDefault()` unconditionally
   
   This blocks ALL touch gestures from reaching the browser, including Chrome's swipe navigation.

### Why Safari iPad Had Issues

Previous fixes likely removed `overscroll-behavior: none` globally, which caused:
- Rubber-band scrolling/bouncing on overscroll
- Edge swipe causing the whole page to slide
- Pull-to-refresh interfering with the app

## Solution Design

### Approach: Scoped Prevention

Keep global `overscroll-behavior: none` but scope touch `preventDefault()` to ONLY the stream viewer canvas element.

### Key Insight

Chrome's swipe-to-navigate works on **overscroll** at page edges, not on individual elements. The issue is:
- `event.preventDefault()` on touch events stops the gesture from propagating
- This happens on the canvas even when the user is trying to navigate at a page level

### Implementation

1. **Keep global `overscroll-behavior: none`** - This is correct for Safari iPad stability

2. **Use CSS `touch-action` on canvas** - Instead of JS `preventDefault()`:
   ```css
   touch-action: none;  /* On the stream viewer canvas only */
   ```
   This tells the browser "don't interpret touch gestures on this element" without blocking gestures elsewhere.

3. **Remove `event.preventDefault()` from stream touch handlers** - Let CSS `touch-action` handle gesture blocking. The canvas already has `touch-action: none` intent through its event handlers.

4. **Alternative (if CSS alone insufficient)**: Only call `preventDefault()` when touch starts INSIDE the canvas bounds. Check `event.target` to ensure we're not blocking gestures that start outside.

## Architecture Decision

**Chosen: CSS `touch-action: none` on canvas**

Rationale:
- Standard browser API for this exact use case
- No JS timing issues or race conditions
- Browser handles gesture disambiguation natively
- Already implicitly expected given our event handlers

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Breaking stream touch controls | `touch-action: none` only disables browser gestures, our JS handlers still fire |
| Safari bounce returning | Keep global `overscroll-behavior: none` |
| Other pages affected | Change is scoped to DesktopStreamViewer only |

## Files to Modify

1. `helix/frontend/src/components/external-agent/DesktopStreamViewer.tsx`
   - Add `touchAction: 'none'` to canvas style
   - Optionally remove redundant `event.preventDefault()` calls (test first)

## Testing Plan

1. Chrome desktop: Two-finger swipe on trackpad on non-stream pages → browser back/forward
2. Chrome desktop: Touch gestures on stream viewer → controls remote desktop
3. Safari iPad: No UI sliding/bouncing when scrolling or interacting
4. Safari iPad: Stream viewer touch controls work
5. Mobile Chrome: Same as desktop Chrome tests

## Implementation Notes

### Discovery (2025-01-XX)

**Current state found:**
- `touchAction: 'none'` is ALREADY present on canvas element (line 3985 in DesktopStreamViewer.tsx)
- `overscroll-behavior: none` is ALREADY set globally in index.html (line 25)
- All touch handlers (handleTouchStart, handleTouchMove, handleTouchEnd, handleTouchCancel) call `event.preventDefault()` unconditionally

**Root cause identified:**
The issue is NOT missing `touchAction: 'none'`. The problem is that `event.preventDefault()` in the touch handlers blocks Chrome's swipe navigation gesture from working, even though the gesture starts outside the canvas.

**Solution:**
Remove `event.preventDefault()` calls from touch handlers. The CSS `touchAction: 'none'` property should be sufficient to prevent browser-default touch handling on the canvas while allowing Chrome's navigation gestures to work elsewhere.

**If preventDefault removal breaks stream controls:**
Implement scoped prevention - only call `preventDefault()` when the touch actually starts within the canvas bounds by checking `event.target`.

## Implementation Summary

**Changes Made:**
1. Removed `event.preventDefault()` from all four touch event handlers:
   - `handleTouchStart` (line ~2956)
   - `handleTouchMove` (line ~3095)
   - `handleTouchEnd` (line ~3237)
   - `handleTouchCancel` (line ~3386)

2. CSS `touchAction: 'none'` remains on canvas element (line 4668) - this is the correct way to prevent default touch behavior

3. Code was auto-formatted by Prettier during save (single quotes → double quotes, reformatted imports)

**Verification:**
- ✅ Frontend tests pass (`yarn test`)
- ✅ Production build succeeds (`yarn build`)
- ⏳ Manual testing required (Chrome desktop, Safari iPad, mobile Chrome)

**Key Insight:**
The issue was that `event.preventDefault()` in JavaScript prevents ALL event propagation, including Chrome's swipe navigation gesture detection. CSS `touchAction: 'none'` only affects the specific element, allowing gestures outside the canvas to work normally.

**Commit:** `b38908070` - "Remove event.preventDefault() from touch handlers"