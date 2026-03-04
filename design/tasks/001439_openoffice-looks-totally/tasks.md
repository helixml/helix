# Implementation Tasks

## Investigation

- [x] Start session at 4K resolution (3840x2160)
- [x] Launch OnlyOffice and confirm only top quarter renders
- [x] Screenshot the broken 4K rendering for documentation
- [x] Identify root cause: XWayland reports 1920x1080 (logical) due to GNOME scaling-factor=2

## Root Cause Analysis (Complete)

- [x] Confirmed OnlyOffice bundles Qt 5.9.9 with X11-only plugins
- [x] Confirmed no Wayland platform plugins (`libqwayland*.so`) exist
- [x] Tested `QT_SCALE_FACTOR=2` - doesn't fix window geometry, just scales content
- [x] Tested `xwayland-native-scaling` experimental feature - this is the fix

## Fix: Enable xwayland-native-scaling (Complete)

- [x] Update `desktop/ubuntu-config/startup-app.sh` to add `xwayland-native-scaling` to experimental features
- [x] Changed gsettings line around line 230:
  ```bash
  gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
  ```
- [x] Commit and push to feature branch

## Testing (Requires Rebuild)

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new 4K session
- [ ] Launch OnlyOffice
- [ ] Verify full window renders (not just top quarter)
- [ ] Screenshot working state

## Cursor Theme (Deferred)

- [ ] Add XCURSOR_THEME=Helix-Invisible to OnlyOffice wrapper
- [ ] Test if OnlyOffice respects system cursor theme
- [ ] Document any limitations (app-rendered cursors for text editing)

## Documentation

- [x] Update design.md with investigation findings
- [x] Document root cause (XWayland + GNOME scaling mismatch)
- [x] List solution options with complexity/effectiveness tradeoffs