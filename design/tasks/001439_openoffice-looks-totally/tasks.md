# Implementation Tasks

## Investigation (Complete)

- [x] Start session at 4K resolution (3840x2160)
- [x] Launch OnlyOffice and confirm only top quarter renders
- [x] Screenshot the broken 4K rendering for documentation
- [x] Identify root cause: XWayland reports 1920x1080 (logical) due to GNOME scaling-factor=2

## Root Cause Analysis (Complete)

- [x] Confirmed OnlyOffice bundles Qt 5.9.9 with X11-only plugins
- [x] Confirmed no Wayland platform plugins (`libqwayland*.so`) exist
- [x] Tested `QT_SCALE_FACTOR=2` - doesn't fix window geometry, just scales content
- [x] Tested `xwayland-native-scaling` experimental feature - needs Mutter restart

## Potential Fixes (Not Yet Implemented)

### Option A: Build OnlyOffice with Qt Wayland Support
- [ ] Clone OnlyOffice desktop-apps repo
- [ ] Set up build environment with Qt 5.15+ and qtwayland5
- [ ] Modify build config to include Wayland platform plugins
- [ ] Build and test
- [ ] Integrate into Dockerfile.ubuntu-helix

### Option B: Try Flatpak Version
- [ ] Install Flatpak OnlyOffice in test environment
- [ ] Check if Flatpak version includes Wayland support
- [ ] Test at 4K resolution
- [ ] If works, update Dockerfile to use Flatpak instead of .deb

### Option C: Enable xwayland-native-scaling at Startup
- [ ] Add to startup-app.sh before gnome-shell starts:
  ```bash
  gsettings set org.gnome.mutter experimental-features "['scale-monitor-framebuffer', 'xwayland-native-scaling']"
  ```
- [ ] Test if this makes XWayland report physical resolution
- [ ] Verify OnlyOffice renders correctly

### Option D: Don't Use Compositor Scaling at 4K
- [ ] Modify startup-app.sh to not set scaling-factor=2 for 4K
- [ ] Instead use GDK_SCALE/QT_SCALE_FACTOR for native Wayland apps only
- [ ] Test impact on other X11 apps

## Cursor Theme (Deferred)

- [ ] Add XCURSOR_THEME=Helix-Invisible to OnlyOffice wrapper
- [ ] Test if OnlyOffice respects system cursor theme
- [ ] Document any limitations (app-rendered cursors for text editing)

## Documentation

- [x] Update design.md with investigation findings
- [x] Document root cause (XWayland + GNOME scaling mismatch)
- [x] List solution options with complexity/effectiveness tradeoffs