# Requirements: Diff View Mobile Usability

## Problem Statement

The diff viewer component (`DiffViewer.tsx`) is unusable on mobile devices. The current desktop-oriented layout with a fixed 280px file list sidebar, side-by-side panels, and wide line number gutters makes reviewing code changes impossible on narrow screens.

## User Stories

1. **As a mobile user**, I want to view code changes on my phone so I can review diffs while away from my desk.

2. **As a mobile user**, I want to navigate between changed files easily without the file list taking up half my screen.

3. **As a mobile user**, I want to read diff content without horizontal scrolling on every line.

## Acceptance Criteria

### Layout
- [ ] On screens < 600px (sm breakpoint), display file list and diff content as separate views, not side-by-side
- [ ] Provide a way to toggle between file list and diff content on mobile
- [ ] File list should be full-width when visible on mobile
- [ ] Diff content should be full-width when visible on mobile

### Line Numbers
- [ ] On mobile, show only new line numbers (or combined single column) instead of dual old/new columns
- [ ] Line number column should be narrower on mobile (e.g., 32px instead of 44px)

### File List
- [ ] Selected file should be clearly visible when returning to file list
- [ ] Tapping a file switches to diff content view automatically on mobile

### Diff Content
- [ ] File path header should truncate gracefully on narrow screens
- [ ] Code lines should wrap appropriately without requiring horizontal scroll
- [ ] +/- indicators should remain visible and not be cut off

### General
- [ ] Desktop experience (screens >= 600px) should remain unchanged
- [ ] Smooth transitions between mobile views
- [ ] Touch targets should be adequately sized (min 44px height for list items)