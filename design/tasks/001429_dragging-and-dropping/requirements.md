# Requirements: iPad Trackpad Drag-and-Drop for Split Screen Tabs

## Problem Statement

Tab drag-and-drop in the split screen view (`TabsView.tsx`) doesn't work on iPad with Magic Keyboard/trackpad. The mousedown/mousemove events don't register when using the trackpad.

## Root Cause

The current implementation uses:
1. **HTML5 Drag and Drop API** (`draggable`, `onDragStart`, `onDragOver`, `onDrop`) - Safari on iPadOS doesn't properly support this with trackpad input
2. **Touch events** (`onTouchStart`, `onTouchMove`, `onTouchEnd`) - These only fire for finger touch, not trackpad

iPadOS Magic Keyboard/trackpad generates **Pointer Events**, not mouse events or touch events. The codebase has no `onPointerDown`, `onPointerMove`, `onPointerUp` handlers for tab dragging.

## User Stories

### US-1: Drag tabs between panels with trackpad
**As** an iPad user with Magic Keyboard  
**I want** to drag tabs between split screen panels using my trackpad  
**So that** I can reorganize my workspace without touching the screen

### US-2: Split panels via drag with trackpad
**As** an iPad user with Magic Keyboard  
**I want** to drag a tab to the edge of a panel to create a new split  
**So that** I can create new split screen layouts

### US-3: Maintain existing functionality
**As** a desktop or touch user  
**I want** drag-and-drop to continue working as before  
**So that** existing workflows aren't broken

## Acceptance Criteria

- [ ] Tabs can be dragged using iPad trackpad (pointer events)
- [ ] Tabs can be dropped on different panels to move them
- [ ] Tabs can be dropped on panel edges to create splits
- [ ] Visual feedback (drag indicators, drop zones) works with trackpad
- [ ] Desktop mouse drag-and-drop still works
- [ ] Touch finger drag-and-drop still works
- [ ] No regression in existing functionality

## Out of Scope

- Keyboard shortcuts for tab management
- Touch gestures beyond existing functionality
- Other drag-and-drop features outside TabsView