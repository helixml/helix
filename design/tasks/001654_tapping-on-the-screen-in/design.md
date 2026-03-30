# Design: Fix Virtual Trackpad Tap Position

## Root Cause

**File:** `frontend/src/components/external-agent/DesktopStreamViewer.tsx`

The bug is a **ref/state desync** in the virtual cursor initialization path.

### How virtual trackpad cursor tracking works

- `cursorPositionRef` (line 175) — a ref holding the virtual cursor's container-relative `{x, y}`. Used synchronously in tap handlers.
- `setCursorPosition` — React state version of the same value. Used for rendering.
- Both are kept in sync during `handleTouchMove` (lines 3229-3239): the ref is updated first for immediate use, state is updated for rendering.

### The bug: initialization only updates React state, not the ref

In `handleTouchStart` (lines 3031-3039), when `!hasMouseMoved`, the cursor is initialized to stream center:

```typescript
if (!hasMouseMoved && containerRef.current) {
  setCursorPosition({
    x: rect.x - containerRect.x + rect.width / 2,
    y: rect.y - containerRect.y + rect.height / 2,
  });
  setHasMouseMoved(true);
}
```

**Problem:** `setCursorPosition` (React state) is updated, but `cursorPositionRef.current` (the ref) is NOT. `cursorPositionRef.current` stays at its initial value of `{x: 0, y: 0}`.

### Where taps go wrong

When a tap is detected in `handleTouchEnd`, `sendCursorPositionToRemote()` (lines 3467-3478) reads `cursorPositionRef.current` to get the click position:

```typescript
const currentPos = cursorPositionRef.current;  // still {x:0, y:0} on first tap!
const streamRelativeX = currentPos.x - streamOffsetX;
const streamRelativeY = currentPos.y - streamOffsetY;
handler.sendMousePosition?.(streamX, streamY, width, height);
```

On first tap (before any drag), `cursorPositionRef.current` is `{x:0, y:0}`. `streamOffsetX` and `streamOffsetY` are the stream's offset within the container (e.g., ~100px if centered). This produces a negative or near-zero `streamRelativeX/Y`, causing the click to land at the top-left corner of the remote screen instead of the center where the cursor is visually shown.

### Fix

In the `handleTouchStart` initialization block, update `cursorPositionRef.current` to match what's being set in React state:

```typescript
if (!hasMouseMoved && containerRef.current) {
  const containerRect = containerRef.current.getBoundingClientRect();
  const centerX = rect.x - containerRect.x + rect.width / 2;
  const centerY = rect.y - containerRect.y + rect.height / 2;
  setCursorPosition({ x: centerX, y: centerY });
  cursorPositionRef.current = { x: centerX, y: centerY };  // ADD THIS
  setHasMouseMoved(true);
}
```

This is the same pattern already used elsewhere in the codebase (e.g., line 1601 where `cursorPositionRef.current` is explicitly set alongside `setCursorPosition`).

## Codebase Patterns Found

- This project uses `useRef` alongside `useState` for cursor position to avoid stale closure issues in event handlers — the ref gives synchronous read of current position.
- The `trackpadCursorRef` DOM element is also updated directly (bypassing React) for 60fps cursor movement (line 3233-3235). For consistency, the DOM update should also happen in the initialization block if the cursor element exists.
- `sendCursorPositionToRemote` is defined inside `handleTouchEnd` as a closure — it captures `rect` from `getStreamRect()` at that point, which is correct.

## Scope

Only `DesktopStreamViewer.tsx` needs to change. One-line fix + containerRect extraction already partially done in the surrounding code. No backend changes needed.
