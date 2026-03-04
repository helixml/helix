# Requirements: OnlyOffice Rendering Issues on GNOME Headless Wayland

## Problem Statement

OnlyOffice Desktop Editors has two issues in the Helix desktop environment:
1. **Broken at 4K resolution** - Only top quarter of screen renders; works fine at 1080p
2. **Renders its own cursor** - Doesn't use system cursor theme (Helix-Invisible), causing duplicate cursors

## User Stories

### US-1: 4K Resolution Support
**As** an AI agent or user with a high-resolution display,
**I want** OnlyOffice to render correctly at 4K resolution,
**So that** I can use the full screen for document editing.

**Acceptance Criteria:**
- [ ] OnlyOffice renders full window at 3840x2160 resolution
- [ ] OnlyOffice renders full window at 2560x1440 resolution
- [ ] Window resizing works at all resolutions
- [ ] No partial rendering or visual corruption at high DPI

### US-2: System Cursor Theme
**As** a user viewing the desktop stream,
**I want** OnlyOffice to use the system cursor theme (Helix-Invisible),
**So that** only the client-side rendered cursor is visible (no duplicates).

**Acceptance Criteria:**
- [ ] OnlyOffice uses Helix-Invisible cursor theme
- [ ] No application-rendered cursor visible in OnlyOffice
- [ ] Cursor shape changes are captured by Helix cursor tracking system

## Technical Context

### Environment
- **Desktop**: Ubuntu 25.10 with GNOME 49 in headless mode
- **Display**: Pure Wayland (no XWayland)
- **Architecture**: AMD64 (OnlyOffice is AMD64-only)
- **Cursor Theme**: Helix-Invisible (transparent cursors with hotspot fingerprinting)
- **App Type**: Electron-based application

### Observed Behavior
- **1080p**: Works correctly
- **4K**: Only top quarter renders (geometry/scaling issue)
- **Cursor**: OnlyOffice ignores system cursor theme, renders its own

### Likely Root Causes
1. **4K Issue**: Electron/Chromium HiDPI scaling misconfiguration in headless Wayland
2. **Cursor Issue**: OnlyOffice not respecting `XCURSOR_THEME` or GTK cursor settings

## Out of Scope
- LibreOffice issues (ARM64 only, separate problem)
- 1080p resolution (already working)

## Dependencies
- GNOME headless mode working correctly
- Helix-Invisible cursor theme installed
- Client-side cursor rendering functional