# Design: iPad Trackpad Drag-and-Drop for Split Screen Tabs

## Overview

Fix tab drag-and-drop in `TabsView.tsx` to work with iPad Magic Keyboard/trackpad by adding Pointer Events support alongside existing HTML5 Drag API and Touch events.

## Technical Analysis

### Current Implementation

The `PanelTab` component in `helix/frontend/src/components/tasks/TabsView.tsx` uses:

```tsx
// HTML5 Drag API (works: desktop browsers; broken: Safari/iPadOS trackpad)
<Box
  draggable
  onDragStart={(e) => onDragStart(e, tab.id)}
  // ...
  onTouchStart={handleTouchStart}  // Touch events (finger only)
  onTouchMove={handleTouchMove}
  onTouchEnd={handleTouchEnd}
>
```

### Why It Fails on iPad Trackpad

| Input Method | Events Fired | Current Support |
|--------------|--------------|-----------------|
| Desktop mouse | Mouse + Drag | ✅ Works |
| Touch finger | Touch | ✅ Works |
| iPad trackpad | Pointer | ❌ Not handled |

Safari on iPadOS fires Pointer Events for trackpad, but the HTML5 Drag API doesn't work properly with them. The `draggable` attribute and `onDragStart` are effectively ignored.

## Solution: Pointer Event Fallback

Add pointer event handlers that implement drag behavior manually when HTML5 Drag API fails.

### Approach

1. **Add pointer event handlers** to `PanelTab` component
2. **Track pointer state** (dragging, start position, current position)
3. **Lift drag state** to parent for cross-panel coordination
4. **Create visual drag indicator** (ghost element during drag)

### Key Changes

#### 1. PanelTab Component

```tsx
// New pointer event handlers
const handlePointerDown = (e: React.PointerEvent) => {
  // Only handle trackpad/mouse pointer types
  if (e.pointerType === 'touch') return; // Let touch handlers handle this
  
  pointerStartRef.current = { x: e.clientX, y: e.clientY };
  // Don't start drag immediately - wait for movement threshold
};

const handlePointerMove = (e: React.PointerEvent) => {
  if (!pointerStartRef.current) return;
  
  const dx = e.clientX - pointerStartRef.current.x;
  const dy = e.clientY - pointerStartRef.current.y;
  const distance = Math.sqrt(dx * dx + dy * dy);
  
  if (distance > 8 && !isPointerDragging) {
    setIsPointerDragging(true);
    onPointerDragStart(tab.id, e.clientX, e.clientY);
    e.currentTarget.setPointerCapture(e.pointerId);
  } else if (isPointerDragging) {
    onPointerDragMove(e.clientX, e.clientY);
  }
};

const handlePointerUp = (e: React.PointerEvent) => {
  if (isPointerDragging) {
    onPointerDragEnd(tab.id, e.clientX, e.clientY);
    e.currentTarget.releasePointerCapture(e.pointerId);
  }
  // Reset state
};
```

#### 2. TaskPanel/TabsView State

```tsx
// Pointer drag state (similar to existing touchDragInfo)
const [pointerDragInfo, setPointerDragInfo] = useState<{
  panelId: string;
  tabId: string;
  currentX: number;
  currentY: number;
} | null>(null);
```

#### 3. Visual Feedback

Render a drag ghost when `pointerDragInfo` is set:

```tsx
{pointerDragInfo && (
  <Box
    sx={{
      position: 'fixed',
      left: pointerDragInfo.currentX,
      top: pointerDragInfo.currentY,
      transform: 'translate(-50%, -50%)',
      pointerEvents: 'none',
      opacity: 0.8,
      zIndex: 9999,
      // Tab styling
    }}
  >
    {/* Tab content preview */}
  </Box>
)}
```

### Alternative Considered: Replace with @dnd-kit

The codebase already uses `@dnd-kit` for other drag operations (`RobustPromptInput`, `AgentKanbanBoard`). However:

- **Pros**: `@dnd-kit` handles pointer events automatically via its `PointerSensor`
- **Cons**: Significant refactor, different API patterns, potential regressions

**Decision**: Add pointer events to existing implementation - minimal change, consistent with touch handling pattern already in place.

## Files Modified

| File | Changes |
|------|---------|
| `helix/frontend/src/components/tasks/TabsView.tsx` | Add pointer event handlers to `PanelTab`, lift pointer drag state, add drag ghost overlay |

## Testing Strategy

1. **iPad + Magic Keyboard**: Primary test - drag tabs between panels, create splits
2. **Desktop browser**: Verify mouse drag still works (HTML5 Drag API)
3. **iPad touch**: Verify finger drag still works (Touch events)
4. **Edge cases**: Rapid drags, cancel mid-drag, drag to same panel

## Risks

- **Pointer capture**: Must properly capture/release to avoid stuck drags
- **Event overlap**: Need to ensure pointer events don't interfere with HTML5 Drag API on desktop (check `pointerType`)
- **Performance**: Drag ghost rendering should be lightweight