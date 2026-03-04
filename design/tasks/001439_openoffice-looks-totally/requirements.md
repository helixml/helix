# Requirements: LibreOffice Rendering Issues on GNOME Headless Wayland

## Problem Statement

LibreOffice renders incorrectly in the Helix desktop environment:
1. **Duplicate mouse cursors** - Multiple cursor sprites visible
2. **Partial rendering** - Only top quarter of screen shows content
3. **Visual corruption** - Generally "totally fucked" appearance

## User Stories

### US-1: Office Document Editing
**As** an AI agent or user working with documents,
**I want** LibreOffice to render correctly in the Helix desktop,
**So that** I can edit Word, Excel, and PowerPoint files on ARM64 systems.

**Acceptance Criteria:**
- [ ] LibreOffice Writer renders full window content
- [ ] LibreOffice Calc renders full spreadsheet area
- [ ] LibreOffice Impress renders full presentation
- [ ] Single cursor visible (no duplicates)
- [ ] Window resizing works correctly
- [ ] Menus and dialogs render properly

### US-2: Cursor Consistency
**As** a user viewing the desktop stream,
**I want** to see only one cursor,
**So that** I can track mouse position accurately.

**Acceptance Criteria:**
- [ ] No duplicate cursors from LibreOffice's internal cursor rendering
- [ ] Cursor shape changes correctly (pointer, text, resize handles)
- [ ] Helix invisible cursor theme works with LibreOffice

## Technical Context

### Environment
- **Desktop**: Ubuntu 25.10 with GNOME 49 in headless mode
- **Display**: Pure Wayland (no XWayland)
- **Architecture**: ARM64 (LibreOffice used instead of OnlyOffice)
- **Cursor**: Helix-Invisible theme with client-side rendering

### Likely Root Causes
1. **VCL Backend Mismatch**: LibreOffice may be using X11/GTK backend expecting XWayland
2. **Missing SAL_USE_VCLPLUGIN**: LibreOffice environment variable not set for Wayland
3. **Headless Mode Incompatibility**: LibreOffice may not support GNOME headless virtual monitor
4. **Cursor Theme Conflict**: LibreOffice's internal cursor may conflict with Helix-Invisible theme

## Out of Scope
- OnlyOffice issues (AMD64 only, not affected)
- Other office suites
- Non-ARM64 platforms

## Dependencies
- GNOME headless mode working correctly
- PipeWire video capture functional
- Client-side cursor rendering working