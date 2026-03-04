# Implementation Tasks

## Core Pointer Event Support

- [~] Add pointer event state to `PanelTab` component (`isPointerDragging`, `pointerStartRef`)
- [~] Implement `handlePointerDown` - capture start position, filter out touch pointerType
- [ ] Implement `handlePointerMove` - detect drag threshold (8px), call `onPointerDragStart`/`onPointerDragMove`
- [ ] Implement `handlePointerUp` - call `onPointerDragEnd`, reset state
- [ ] Add pointer event props to `PanelTab` JSX (`onPointerDown`, `onPointerMove`, `onPointerUp`, `onPointerCancel`)
- [ ] Use `setPointerCapture`/`releasePointerCapture` to track drag across elements

## Parent Component State (TabsView)

- [ ] Add `pointerDragInfo` state to `TabsView` (similar to existing `touchDragInfo`)
- [ ] Add `handlePointerDragStart` callback - set drag info with panelId, tabId, position
- [ ] Add `handlePointerDragMove` callback - update current position in state
- [ ] Add `handlePointerDragEnd` callback - find target panel at drop position, call `handleDropTab`
- [ ] Pass pointer drag callbacks through `TaskPanel` to `PanelTab`

## Visual Feedback

- [ ] Add drag ghost overlay to `TabsView` render when `pointerDragInfo` is set
- [ ] Style ghost to match tab appearance (position: fixed, follows cursor)
- [ ] Add opacity to dragged tab while dragging
- [ ] Ensure drop zone indicators work with pointer drag (reuse existing `dragOverEdge` logic)

## Interface Updates

- [ ] Update `PanelTabProps` interface with new pointer drag callbacks
- [ ] Update `TaskPanelProps` interface to pass through pointer drag callbacks

## Testing

- [ ] Test on iPad with Magic Keyboard trackpad - drag between panels
- [ ] Test on iPad with Magic Keyboard trackpad - drag to panel edge for split
- [ ] Test desktop mouse drag still works (HTML5 Drag API)
- [ ] Test iPad finger touch drag still works (Touch events)
- [ ] Test cancel drag mid-operation (press Escape or move outside window)