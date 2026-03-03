# Design: Diff View Mobile Usability

## Overview

Make the diff viewer component responsive by implementing a stacked view layout for mobile screens, replacing the side-by-side file list + diff content layout.

## Current Architecture

```
DiffViewer.tsx
├── Header (title, refresh button, branch info)
├── Tabs (workspace/helix-specs tabs)
└── Content area (flex row)
    ├── File list sidebar (fixed 280px width)
    │   └── DiffFileList.tsx
    └── Diff content panel (flex: 1)
        └── DiffContent.tsx
            └── Line rows with dual line number columns (44px each)
```

## Proposed Solution

### Mobile Layout (< 600px / sm breakpoint)

Switch from side-by-side to a **stacked tab-based layout**:

```
DiffViewer.tsx
├── Header (unchanged)
├── Tabs (unchanged)
├── Mobile sub-tabs: [Files] [Diff]  ← NEW
└── Content area (full width)
    └── Either DiffFileList OR DiffContent (not both)
```

### Key Changes

1. **DiffViewer.tsx**
   - Add `useIsBigScreen({ breakpoint: 'sm' })` hook
   - Add local state `mobileView: 'files' | 'diff'`
   - On mobile: render toggle buttons + single view
   - On desktop: keep existing side-by-side layout
   - Auto-switch to 'diff' view when file is selected on mobile

2. **DiffContent.tsx**
   - Add "Back to files" button on mobile (visible when prop passed)
   - Reduce line number column width: `{ xs: 32, sm: 44 }px`
   - Show single line number column on mobile (new line number only)
   - File path truncation with ellipsis

3. **DiffFileList.tsx**
   - No structural changes needed
   - Already full-width when parent container is full-width

### Component Interface Changes

```typescript
// DiffContent.tsx - add optional props
interface DiffContentProps {
  file: FileDiff | null
  isLoading?: boolean
  onCopyPath?: () => void
  onBack?: () => void      // NEW: callback to return to file list (mobile)
  isMobile?: boolean       // NEW: enables mobile-specific styling
}
```

## Design Decisions

### Why tabs instead of drawer/modal?
- Consistent with existing mobile patterns in `SpecTaskDetailContent.tsx`
- Simpler implementation, no animation complexity
- Clear navigation model users already understand

### Why show new line numbers only on mobile?
- Old line numbers rarely needed for quick reviews
- Saves ~50px horizontal space per line
- Similar to GitHub mobile behavior

### Why 600px (sm) breakpoint?
- MUI standard breakpoint, already used elsewhere
- Matches typical phone portrait width threshold
- Tablets in landscape get desktop experience

## Files to Modify

| File | Change |
|------|--------|
| `DiffViewer.tsx` | Add responsive layout logic, mobile view state |
| `DiffContent.tsx` | Add back button, responsive line numbers |
| `DiffFileList.tsx` | Minor: ensure touch targets are 44px+ |

## Implementation Notes

### What Was Implemented

1. **DiffViewer.tsx changes:**
   - Added `useIsBigScreen` hook with `sm` breakpoint (600px)
   - Added `mobileView` state toggling between `'files'` and `'diff'`
   - Added ToggleButtonGroup with Files/Diff buttons using lucide icons
   - Separate rendering paths for desktop (side-by-side) and mobile (stacked)
   - Auto-switch to diff view on file selection (`handleSelectFile`)
   - Reset to files view when switching workspace tabs
   - Hidden branch info on mobile header to save space

2. **DiffContent.tsx changes:**
   - Added `onBack` and `isMobile` props to interface
   - Added back arrow button in header when `onBack` is provided
   - Responsive line number column: 32px on mobile, 44px on desktop
   - Hide old line number column on mobile (show combined line number)
   - Smaller font sizes on mobile (0.65rem vs 0.72rem)
   - File path truncation with text-overflow ellipsis
   - Hide copy path button on mobile to save space
   - Reduced padding on mobile for more content space

3. **DiffFileList.tsx changes:**
   - Added `minHeight: 44` to ListItemButton for touch accessibility
   - Increased vertical padding (`py: 1` instead of `py: 0.75`)

### Patterns Used

- Used existing `useIsBigScreen` hook pattern from `SpecTaskDetailContent.tsx`
- Used `ToggleButtonGroup` pattern consistent with mobile view toggles elsewhere
- Followed existing responsive styling conventions with MUI sx prop

### Testing Notes

- Build verified with `yarn build` - compiles successfully
- Manual testing recommended on mobile viewport (375px width)
- Test flow: tap file → view diff → tap back → file list
- Desktop layout should remain unchanged