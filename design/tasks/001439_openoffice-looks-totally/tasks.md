# Implementation Tasks

## Investigation (Complete)

- [x] Start session at 4K resolution (3840x2160)
- [x] Launch OnlyOffice and confirm only top quarter renders
- [x] Screenshot the broken 4K rendering for documentation
- [x] Identify root cause: OnlyOffice is Qt5+CEF (not Electron), X11 only, no Wayland support
- [x] Confirm XWayland reports logical resolution (1920x1080) not physical (3840x2160)
- [x] Test QT_SCALE_FACTOR - doesn't fix window geometry issue
- [x] Test QT_SCREEN_SCALE_FACTORS - doesn't fix window geometry issue

## Root Cause

OnlyOffice bundles Qt 5.9 with only X11 support. When GNOME runs at 4K with 2x scaling:
- XWayland reports 1920x1080 (logical) to X11 apps
- OnlyOffice creates 1920x1080 window
- Window surface is actually 3840x2160
- Result: content in top-left quarter only

## Fix: Enable XWayland Native Scaling

- [ ] Update `desktop/ubuntu-config/startup-app.sh` to add `xwayland-native-scaling` to experimental features BEFORE gnome-shell starts
- [ ] Add to the gsettings line around line 230:
  ```bash
  gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
  ```

## Fix: OnlyOffice Wrapper Script

- [ ] Update Dockerfile.ubuntu-helix to add cursor theme env vars to OnlyOffice wrapper:
  ```bash
  export XCURSOR_THEME=Helix-Invisible
  export XCURSOR_SIZE=48
  ```

## Testing

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new 4K session
- [ ] Launch OnlyOffice
- [ ] Verify full window renders (not just top quarter)
- [ ] Verify cursor uses Helix-Invisible theme
- [ ] Screenshot working state

## Documentation

- [x] Update design.md with actual root cause (Qt5+CEF, not Electron)
- [x] Document XWayland scaling behavior
- [ ] Commit and push changes to helix-specs