# Requirements: OnlyOffice Rendering Issues on GNOME Headless Wayland

## Problem Statement

OnlyOffice Desktop Editors renders incorrectly in the Helix desktop environment:
1. **Duplicate mouse cursors** - Multiple cursor sprites visible
2. **Partial rendering** - Only top quarter of screen shows content
3. **Visual corruption** - Generally broken appearance

## User Stories

### US-1: Office Document Editing
**As** an AI agent or user working with documents,
**I want** OnlyOffice to render correctly in the Helix desktop,
**So that** I can edit Word, Excel, and PowerPoint files.

**Acceptance Criteria:**
- [ ] OnlyOffice Document Editor renders full window content
- [ ] OnlyOffice Spreadsheet Editor renders full spreadsheet area
- [ ] OnlyOffice Presentation Editor renders full presentation
- [ ] Single cursor visible (no duplicates)
- [ ] Window resizing works correctly
- [ ] Menus and dialogs render properly

### US-2: Cursor Consistency
**As** a user viewing the desktop stream,
**I want** to see only one cursor,
**So that** I can track mouse position accurately.

**Acceptance Criteria:**
- [ ] No duplicate cursors from OnlyOffice's internal cursor rendering
- [ ] Cursor shape changes correctly (pointer, text, resize handles)
- [ ] Helix invisible cursor theme works with OnlyOffice

## Technical Context

### Environment
- **Desktop**: Ubuntu 25.10 with GNOME 49 in headless mode
- **Display**: Pure Wayland (no XWayland)
- **Architecture**: AMD64 (OnlyOffice is AMD64-only; ARM64 uses LibreOffice)
- **Cursor**: Helix-Invisible theme with client-side rendering
- **App Type**: Electron-based application

### Likely Root Causes
1. **Electron/Wayland Misconfiguration**: OnlyOffice (Electron) may need `--ozone-platform=wayland` flag
2. **Missing Electron Wayland flags**: Similar to Chrome wrapper needing special flags
3. **Headless Mode Incompatibility**: Electron apps may not properly detect virtual monitor geometry
4. **Cursor Theme Conflict**: OnlyOffice's internal cursor may conflict with Helix-Invisible theme

### Reference: Chrome Wrapper Pattern
Chrome already has a wrapper script with Wayland-compatible flags:
```bash
exec /usr/bin/google-chrome-stable.real --password-store=basic --disable-dev-shm-usage --enable-features=VaapiVideoDecoder,VaapiVideoDecodeLinuxGL --disable-features=UseChromeOSDirectVideoDecoder "$@"
```

OnlyOffice likely needs similar treatment.

## Out of Scope
- LibreOffice issues (ARM64 only, separate problem)
- Non-Electron office suites
- ARM64 platform (uses LibreOffice instead)

## Dependencies
- GNOME headless mode working correctly
- PipeWire video capture functional
- Client-side cursor rendering working