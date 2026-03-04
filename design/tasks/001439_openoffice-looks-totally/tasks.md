# Implementation Tasks

## Investigation

- [x] Start session at 4K resolution (3840x2160)
- [x] Launch OnlyOffice and confirm only top quarter renders
- [x] Screenshot the broken 4K rendering for documentation
- [x] Verify it works correctly at 1080p (baseline)

**Finding**: OnlyOffice uses Qt5 + CEF (Chromium Embedded Framework), NOT native Wayland. It bundles its own Qt without the Wayland plugin - only has xcb (X11). It REQUIRES XWayland to run.

**Root Cause of 4K Issue**: GNOME at 2x scaling reports 1920x1080 logical resolution to XWayland. OnlyOffice renders at that size, filling only top-left quarter of 4K physical display. This is XWayland behavior, not an OnlyOffice bug.

**Cursor Theme**: XCURSOR_THEME=Helix-Invisible DOES work - cursor becomes invisible as expected.

## Fix: Wrapper Script

- [~] Add wrapper script to `Dockerfile.ubuntu-helix` after OnlyOffice install (~line 365):
  ```bash
  #!/bin/bash
  # OnlyOffice requires X11 via XWayland (Qt5 without Wayland plugin)
  export DISPLAY=:0
  
  # Find the Mutter XWayland auth file (name changes on each session)
  XAUTH_FILE=$(ls /run/user/1000/.mutter-Xwaylandauth.* 2>/dev/null | tail -1)
  if [ -n "$XAUTH_FILE" ]; then
      export XAUTHORITY="$XAUTH_FILE"
  fi
  
  # Cursor theme for X11 apps
  export XCURSOR_THEME=Helix-Invisible
  export XCURSOR_SIZE=48
  
  # Required library path
  APP_PATH=/opt/onlyoffice/desktopeditors
  export LD_LIBRARY_PATH=$APP_PATH${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
  
  exec $APP_PATH/DesktopEditors "$@"
  ```

## Testing

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new session
- [ ] Launch OnlyOffice via menu or `desktopeditors` command
- [ ] Verify OnlyOffice launches without errors
- [ ] Verify cursor is invisible (using Helix-Invisible theme)

## Documentation

- [x] Update design.md with actual root cause findings
- [ ] Document 4K scaling as known limitation (XWayland reports logical resolution)