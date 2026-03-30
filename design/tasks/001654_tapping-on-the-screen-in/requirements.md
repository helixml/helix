# Requirements: Fix Virtual Trackpad Tap Position

## User Story

As a user in virtual trackpad mode, when I tap the screen, I expect the click to be sent to where the virtual cursor (the pointer on the remote screen) is currently positioned — not to some other position.

## Problem Statement

Tapping on the screen in virtual trackpad mode sends the click to the wrong position. The click goes to wherever `cursorPositionRef.current` happens to be (often `{x:0, y:0}` on first tap, or a stale position), rather than where the virtual cursor is visually displayed.

## Acceptance Criteria

1. **First tap works correctly**: When a user taps without having dragged first, the click is sent to the stream center (where the virtual cursor is initially rendered), not to `(0, 0)` / top-left of the remote screen.
2. **Subsequent taps work correctly**: After dragging the cursor to a position, tapping sends the click to that dragged-to position.
3. **Multi-finger taps work correctly**: Two-finger (right-click) and three-finger (middle-click) taps also use the correct cursor position.
4. **No regression in drag behavior**: Double-tap-drag and scrolling continue to work as before.
