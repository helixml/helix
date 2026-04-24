# Requirements

## User Story

As a mobile user using the virtual trackpad mode, I want a single tap to produce exactly one click on the remote desktop, so that I can interact with menus, buttons, and other UI elements without them toggling on and off.

## Problem

Tapping in trackpad mode sends two clicks. This makes the trackpad unusable — tapping a menu opens then immediately closes it, tapping a button activates then deactivates it, etc.

## Acceptance Criteria

- [ ] A single tap in trackpad mode sends exactly one left click
- [ ] Two-finger tap still sends exactly one right click
- [ ] Three-finger tap still sends exactly one middle click
- [ ] Double-tap-to-drag still works correctly
- [ ] Mouse events from a real external mouse/trackpad (e.g. Magic Keyboard) still work — only synthetic browser-generated mouse events are suppressed
- [ ] No regression in "direct" touch mode
