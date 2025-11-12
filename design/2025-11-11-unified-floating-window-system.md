# Unified Floating Window System

**Date:** 2025-11-11
**Status:** 📋 PLANNED
**Effort:** Medium (12-17 hours)
**Priority:** High - Improves UX consistency and enables cross-window tiling

## Problem Statement

We currently have multiple floating window implementations across the codebase:

1. **FloatingModal.tsx** - Context-based, supports logs/RDP/exploratory sessions
2. **SpecTaskDetailDialog.tsx** - Props-based, supports agent/task cards

Both implementations now have similar features (resize, tile, snap, drag) after recent improvements, but they:
- Cannot interact with each other (no cross-window tiling)
- Have separate z-index management (windows don't coordinate)
- Duplicate ~80% of the same code (resize logic, tile logic, snap detection)
- Make it harder to add new window types (must choose which pattern to follow)
- Inconsistent behavior (drag thresholds, z-index handling, cursor states differ)

**User Impact:**
- Can't tile an agent window next to a log window
- Clicking different window types doesn't bring them all to front correctly
- Inconsistent window behavior based on what opened it

## Current Implementation Analysis

### FloatingModal.tsx (Context-based)
**Location:** `frontend/src/components/admin/FloatingModal.tsx`
**Context:** `frontend/src/contexts/floatingModal.tsx`

```typescript
// Usage pattern:
const floatingModal = useFloatingModal()
floatingModal.showFloatingModal({
  type: 'logs',
  runner: {...}
}, clickPosition)
```

**Features:**
- ✅ Resize handles (useResize hook)
- ✅ Tiling (7 positions: full, half-left, half-right, 4 corners)
- ✅ Snap preview (50px threshold)
- ✅ Drag with threshold (5px normal, 15px when snapped)
- ✅ Z-index: 9999 base, 100000 snap preview, 100001 menu

**Limitations:**
- Only supports 1 window at a time
- Hard-coded content types ('logs' | 'rdp' | 'exploratory_session')
- Global singleton pattern

### SpecTaskDetailDialog.tsx (Props-based)
**Location:** `frontend/src/components/tasks/SpecTaskDetailDialog.tsx`

```typescript
// Usage pattern:
<SpecTaskDetailDialog
  task={task}
  open={open}
  onClose={onClose}
/>
```

**Features:**
- ✅ Resize handles (same useResize hook)
- ✅ Tiling (same 7 positions)
- ✅ Snap preview (same 50px threshold)
- ✅ Drag with threshold (same logic)
- ✅ Tabs (Active Session, Details)
- ✅ Z-index: 9999 base, 100000 snap preview, 100001 menu

**Limitations:**
- Can open multiple windows, but they don't coordinate z-index
- Can't tile with FloatingModal windows
- Duplicates all window management code

### Code Overlap

**Shared patterns:**
1. Window positioning (`{ x, y, width, height }`)
2. Resize logic (`useResize` hook - **already unified!**)
3. Tile positions (enum: 'center' | 'full' | 'half-left' | etc.)
4. Snap detection (50px threshold, 9 snap zones)
5. Drag behavior (threshold-based, position tracking)
6. Z-index management (base + overlays)

**Differences:**
- Content rendering (type-switch vs JSX children)
- Opening mechanism (context vs props)
- Multi-window support (singleton vs multiple)

## Proposed Solution

### Phase 1: Extract Base Component (4-6 hours)

Create `frontend/src/components/system/FloatingWindow.tsx` - a fully-featured, reusable floating window component.

```typescript
interface FloatingWindowProps {
  // Window management
  id: string                          // Unique ID for window
  title: string | ReactNode           // Window title
  open: boolean                       // Visibility
  onClose: () => void                 // Close handler

  // Positioning
  initialPosition?: { x: number; y: number }
  initialSize?: { width: number; height: number }
  minSize?: { width: number; height: number }
  maxSize?: { width: number; height: number }

  // Behavior
  resizable?: boolean                 // Default: true
  tileable?: boolean                  // Default: true
  draggable?: boolean                 // Default: true

  // Z-index coordination
  zIndexBase?: number                 // Default: 9999
  onWindowClick?: (id: string) => void // Called when window clicked

  // Content
  children: ReactNode                 // Window content

  // Header customization
  headerActions?: ReactNode           // Custom buttons in header
  showTileButton?: boolean            // Default: true
  showCloseButton?: boolean           // Default: true
}
```

**Extracted components:**
1. `FloatingWindow` - Main component
2. `useFloatingWindowState` - Hook for position/size/tiling state
3. `ResizeHandles` - Render resize handles (already in `useResize`)
4. `TileMenu` - Tiling menu component
5. `SnapPreview` - Snap preview overlay

### Phase 2: Window Manager Context (6-8 hours)

Create `frontend/src/contexts/windowManager.tsx` - global coordinator for ALL windows.

```typescript
interface WindowManagerContextType {
  // Window registry
  windows: Map<string, WindowState>

  // Registration
  registerWindow: (id: string, state: WindowState) => void
  unregisterWindow: (id: string) => void

  // Z-index coordination
  bringToFront: (id: string) => void
  getZIndex: (id: string) => number

  // Multi-window tiling
  getAllWindowBounds: () => { id: string, bounds: Bounds }[]
  snapToWindow: (sourceId: string, targetId: string, position: SnapPosition) => void

  // Global state
  maxZIndex: number
}

interface WindowState {
  id: string
  position: { x: number; y: number }
  size: { width: number; height: number }
  tilePosition: TilePosition | null
  zIndex: number
  visible: boolean
}
```

**Features:**
- Track all open windows (from any source)
- Global z-index coordination (click any window → brings to front)
- Enable cross-window snapping (agent window can snap to log window)
- Window-to-window alignment helpers
- Prevent window overlap conflicts

### Phase 3: Migration Plan (2-3 hours)

#### 3.1 Migrate FloatingModal
```typescript
// frontend/src/components/admin/FloatingModal.tsx (AFTER)
const FloatingModal: FC<FloatingModalProps> = () => {
  const floatingModal = useFloatingModal()
  const windowManager = useWindowManager()

  if (!floatingModal.isVisible || !floatingModal.modalConfig) return null

  const { modalConfig } = floatingModal

  return (
    <FloatingWindow
      id="floating-modal"
      title={getTitleForType(modalConfig.type)}
      open={floatingModal.isVisible}
      onClose={floatingModal.hideFloatingModal}
      initialPosition={floatingModal.clickPosition}
      onWindowClick={() => windowManager.bringToFront('floating-modal')}
    >
      {/* Render content based on modalConfig.type */}
      {modalConfig.type === 'logs' && <LogViewerModal runner={modalConfig.runner} />}
      {modalConfig.type === 'rdp' && <ScreenshotViewer sessionId={modalConfig.sessionId} />}
      {modalConfig.type === 'exploratory_session' && <ExternalAgentDesktopViewer sessionId={modalConfig.sessionId} />}
    </FloatingWindow>
  )
}
```

#### 3.2 Migrate SpecTaskDetailDialog
```typescript
// frontend/src/components/tasks/SpecTaskDetailDialog.tsx (AFTER)
const SpecTaskDetailDialog: FC<SpecTaskDetailDialogProps> = ({ task, open, onClose }) => {
  const windowManager = useWindowManager()
  const windowId = `spec-task-${task?.id}`

  if (!task) return null

  return (
    <FloatingWindow
      id={windowId}
      title={task.name}
      open={open}
      onClose={onClose}
      initialPosition={{ x: 100, y: 100 }}
      onWindowClick={() => windowManager.bringToFront(windowId)}
      headerActions={
        onEdit && <IconButton onClick={() => onEdit(task)}><EditIcon /></IconButton>
      }
    >
      {/* Tabs and content */}
      <Tabs>...</Tabs>
      <TabContent>...</TabContent>
    </FloatingWindow>
  )
}
```

#### 3.3 Support Multiple Windows of Same Type
Enable scenarios like:
- Two agent windows side-by-side
- Agent window + log window tiled
- Three log windows for different runners

```typescript
// New usage pattern:
floatingModal.showFloatingModal({
  id: `logs-${runner.id}`,  // Unique ID per window
  type: 'logs',
  runner: runner
})
```

## Implementation Phases

### Phase 1: Base Component (4-6 hours)
**Files to create:**
- `frontend/src/components/system/FloatingWindow.tsx`
- `frontend/src/hooks/useFloatingWindowState.ts`
- `frontend/src/components/system/SnapPreview.tsx`
- `frontend/src/components/system/TileMenu.tsx`

**Tasks:**
1. ✅ Extract common window shell (title bar, resize handles, close button)
2. ✅ Integrate useResize hook (already unified)
3. ✅ Implement tiling logic (reuse existing)
4. ✅ Implement snap detection (reuse existing)
5. ✅ Add prop-based customization
6. ✅ Write unit tests for positioning logic

**Validation:**
- Create test page with 2 FloatingWindow instances
- Verify resize, tile, snap, drag all work
- Verify z-index coordination when clicking between windows

### Phase 2: Window Manager (6-8 hours)
**Files to create:**
- `frontend/src/contexts/windowManager.tsx`
- `frontend/src/hooks/useWindowManager.ts`

**Tasks:**
1. ✅ Create window registry (Map<id, WindowState>)
2. ✅ Implement z-index coordination
3. ✅ Add window registration/unregistration
4. ✅ Implement bringToFront (incremental z-index)
5. ✅ Add cross-window snap detection
6. ✅ Prevent window ID collisions
7. ✅ Handle window cleanup on unmount

**Validation:**
- Open 3 windows from different sources
- Click between them → verify correct z-index ordering
- Snap window A to window B → verify alignment
- Close window → verify cleanup

### Phase 3: Migration (2-3 hours)
**Files to modify:**
- `frontend/src/components/admin/FloatingModal.tsx`
- `frontend/src/contexts/floatingModal.tsx` (update types)
- `frontend/src/components/tasks/SpecTaskDetailDialog.tsx`

**Tasks:**
1. ✅ Update FloatingModal to use FloatingWindow
2. ✅ Update SpecTaskDetailDialog to use FloatingWindow
3. ✅ Update floatingModal context to support multiple windows
4. ✅ Remove duplicated code
5. ✅ Test all existing use cases
6. ✅ Update any hardcoded z-index values

**Validation:**
- All existing functionality works unchanged
- No visual regressions
- Cross-window tiling now works
- Z-index coordination works globally

## Benefits

### User Experience
- ✅ **Cross-window tiling**: Tile agent window next to log window
- ✅ **Consistent behavior**: All windows resize/tile/snap the same way
- ✅ **Better z-index**: Click any window → comes to front correctly
- ✅ **Multiple windows**: Open multiple logs, multiple agents simultaneously
- ✅ **Window-to-window snapping**: Snap to existing windows, not just screen edges

### Developer Experience
- ✅ **Single component**: Add new window types with `<FloatingWindow>{content}</FloatingWindow>`
- ✅ **Less duplication**: ~400 lines of duplicate code removed
- ✅ **Easier maintenance**: Fix bugs once, applies everywhere
- ✅ **Testable**: Core logic isolated in testable hooks
- ✅ **Flexible**: Props-based API supports any content type

### Examples of New Capabilities

**Example 1: Tile agent window next to logs**
```
┌─────────────────┬─────────────────┐
│ Agent: Task 123 │ Logs: Runner 45 │
│                 │                 │
│ [streaming...]  │ [log output...] │
│                 │                 │
└─────────────────┴─────────────────┘
```

**Example 2: Multiple log windows**
```
┌──────────────┬──────────────┐
│ Logs: GPU 0  │ Logs: GPU 1  │
├──────────────┴──────────────┤
│ Agent: Planning Session     │
│                              │
└──────────────────────────────┘
```

**Example 3: Window-to-window snapping**
- Drag agent window near log window edge
- Shows snap preview aligned to log window
- Release → agent window snaps perfectly to log window

## Testing Plan

### Unit Tests
- `FloatingWindow` component
  - Resize handle positioning
  - Tile position calculations
  - Snap detection logic
  - Z-index updates
- `useWindowManager` hook
  - Window registration/unregistration
  - Z-index coordination
  - Multiple window support

### Integration Tests
- Open 2 windows from different sources → verify coordination
- Tile window A → open window B → verify no conflicts
- Drag window across snap zones → verify preview accuracy
- Click window stack → verify z-index ordering

### Manual Testing Checklist
- [ ] Open agent window, resize from all 8 handles
- [ ] Open log window, tile to half-left
- [ ] Open agent window, snap to log window edge
- [ ] Click between windows → verify z-index
- [ ] Drag snapped window → verify threshold (15px)
- [ ] Open 3+ windows → verify all tile correctly
- [ ] Close window while others open → verify no crashes
- [ ] Refresh page with windows open → verify cleanup

## Risks & Mitigation

### Risk: Breaking Existing Functionality
**Mitigation:**
- Incremental migration (Phase 1 → 2 → 3)
- Keep old code until new code validated
- Feature flag for new system during testing

### Risk: Z-index Conflicts with Other UI
**Mitigation:**
- Use high z-index base (9999+)
- Document z-index ranges in code
- Add z-index constants file

### Risk: Performance with Many Windows
**Mitigation:**
- Efficient window registry (Map instead of Array)
- Debounce z-index updates
- Lazy render window content
- Virtual rendering for off-screen windows

### Risk: Window Position Persistence
**Decision needed:** Should window positions persist across sessions?
- If yes: Add localStorage serialization
- If no: Always reset to initial position

## Future Enhancements (Out of Scope)

These could be added later without breaking changes:

1. **Window persistence**: Save/restore window positions in localStorage
2. **Window history**: Undo/redo window movements
3. **Keyboard shortcuts**: Alt+Arrow to move windows, Ctrl+Arrow to resize
4. **Window animations**: Smooth transitions when tiling
5. **Window groups**: Link windows together (move as one)
6. **Minimize to taskbar**: Collapsed window bar at bottom
7. **Window templates**: Save/restore window layouts
8. **Mobile support**: Touch-friendly resize handles, swipe gestures

## Code Locations

### New Files (Phase 1)
- `frontend/src/components/system/FloatingWindow.tsx` - Base component
- `frontend/src/hooks/useFloatingWindowState.ts` - Window state hook
- `frontend/src/components/system/SnapPreview.tsx` - Snap preview overlay
- `frontend/src/components/system/TileMenu.tsx` - Tiling menu

### New Files (Phase 2)
- `frontend/src/contexts/windowManager.tsx` - Window manager context
- `frontend/src/hooks/useWindowManager.ts` - Window manager hook

### Modified Files (Phase 3)
- `frontend/src/components/admin/FloatingModal.tsx` - Use FloatingWindow
- `frontend/src/contexts/floatingModal.tsx` - Support multiple windows
- `frontend/src/components/tasks/SpecTaskDetailDialog.tsx` - Use FloatingWindow

### Shared Code (Already Exists)
- `frontend/src/hooks/useResize.ts` - Resize logic (no changes needed)

### Removed After Migration
- ~200 lines from FloatingModal.tsx (window management code)
- ~200 lines from SpecTaskDetailDialog.tsx (window management code)
- Net reduction: ~400 lines of duplicate code

## Success Criteria

✅ **Phase 1 Complete When:**
- FloatingWindow component renders with all features
- Can open 2 test windows, resize, tile, drag both
- No duplicate code between test windows

✅ **Phase 2 Complete When:**
- WindowManager tracks all windows globally
- Clicking any window brings it to front
- Can snap one window to another window's edge

✅ **Phase 3 Complete When:**
- All existing windows migrated to new system
- No visual or functional regressions
- All existing tests pass
- Can tile agent window next to log window

## Timeline Estimate

**Total: 12-17 hours**

- Phase 1: 4-6 hours
- Phase 2: 6-8 hours
- Phase 3: 2-3 hours

**Suggested approach:**
- Do Phase 1 in one session (get base component working)
- Do Phase 2 in next session (add global coordination)
- Do Phase 3 incrementally (migrate one component at a time)

**When to do this:**
- After current sprint
- Low-risk time (not before release)
- When we can dedicate uninterrupted time for testing

## Related Work

- **Recent improvements** (2025-11-11): Both FloatingModal and SpecTaskDetailDialog now have resize handles, proper snap detection, and reduced title bars
- **useResize hook**: Already unified across both components
- **Wolf streaming**: Uses these windows for agent sessions
- **Admin dashboard**: Uses these windows for runner logs

## Questions to Resolve Before Starting

1. **Window persistence**: Should positions save to localStorage?
2. **Max windows**: Set a limit (e.g., 10 max) or unlimited?
3. **Window IDs**: Auto-generate or require explicit IDs?
4. **Feature flag**: Deploy behind flag or ship directly?
5. **Mobile support**: In scope or future enhancement?
6. **Window animations**: Smooth transitions or instant?

## Conclusion

This unification eliminates duplicate code, enables powerful new features (cross-window tiling), and sets up a foundation for future windowing enhancements. The implementation is low-risk because we can do it incrementally and both existing implementations are already very similar.

**Recommendation:** Schedule this work for next sprint when we have time for thorough testing.
