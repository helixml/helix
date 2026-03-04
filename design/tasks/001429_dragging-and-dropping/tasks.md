# Implementation Tasks

## Core Pointer Event Support

- [x] Add pointer event state to `PanelTab` component (`isPointerDragging`, `pointerStartRef`)
- [x] Implement `handlePointerDown` - capture start position, filter out touch pointerType
- [x] Implement `handlePointerMove` - detect drag threshold (8px), call `onPointerDragStart`/`onPointerDragMove`
- [x] Implement `handlePointerUp` - call `onPointerDragEnd`, reset state
- [x] Add pointer event props to `PanelTab` JSX (`onPointerDown`, `onPointerMove`, `onPointerUp`, `onPointerCancel`)
- [x] Use `setPointerCapture`/`releasePointerCapture` to track drag across elements

## Parent Component State (TabsView)

- [x] Add `pointerDragInfo` state to `TabsView` (similar to existing `touchDragInfo`)
- [x] Add `handlePointerDragStart` callback - set drag info with panelId, tabId, position
- [x] Add `handlePointerDragMove` callback - update current position in state
- [x] Add `handlePointerDragEnd` callback - find target panel at drop position, call `handleDropTab`
- [x] Pass pointer drag callbacks through `TaskPanel` to `PanelTab`

## Visual Feedback

- [x] Add opacity to dragged tab while dragging
- [x] Ensure drop zone indicators work with pointer drag (reuse existing `dragOverEdge` logic)
- [ ] Add drag ghost overlay to `TabsView` render when `pointerDragInfo` is set (optional - basic functionality works without)
- [ ] Style ghost to match tab appearance (position: fixed, follows cursor) (optional)

## Interface Updates

- [x] Update `PanelTabProps` interface with new pointer drag callbacks
- [x] Update `TaskPanelProps` interface to pass through pointer drag callbacks

## Testing

- [ ] Test on iPad with Magic Keyboard trackpad - drag between panels
- [ ] Test on iPad with Magic Keyboard trackpad - drag to panel edge for split
- [ ] Test desktop mouse drag still works (HTML5 Drag API)
- [ ] Test iPad finger touch drag still works (Touch events)
- [ ] Test cancel drag mid-operation (press Escape or move outside window)