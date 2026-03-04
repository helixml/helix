# Implementation Tasks

## Investigation

- [x] Start session at 4K resolution (3840x2160)
- [x] Launch OnlyOffice and confirm only top quarter renders
- [x] Screenshot the broken 4K rendering for documentation
- [x] Verify it works correctly at 1080p (baseline)

**Finding**: OnlyOffice uses Qt5 + CEF (Chromium Embedded Framework), NOT native Wayland. It bundles its own Qt without the Wayland plugin - only has xcb (X11). It REQUIRES XWayland to run.

**Current state**: OnlyOffice works when launched with correct environment:
```bash
export XAUTHORITY=/run/user/1000/.mutter-Xwaylandauth.*
export DISPLAY=:0
export LD_LIBRARY_PATH=/opt/onlyoffice/desktopeditors
/opt/onlyoffice/desktopeditors/DesktopEditors
```

## Fix: Wrapper Script

- [ ] Update wrapper script in `Dockerfile.ubuntu-helix` to use XWayland (not Wayland):
  ```bash
  mv /usr/bin/desktopeditors /usr/bin/desktopeditors.real
  cat > /usr/bin/desktopeditors << 'EOF'
  #!/bin/bash
  # OnlyOffice requires X11 via XWayland (Qt5 without Wayland plugin)
  export DISPLAY=:0
  # Find the Mutter XWayland auth file
  XAUTH_FILE=$(ls /run/user/1000/.mutter-Xwaylandauth.* 2>/dev/null | head -1)
  if [ -n "$XAUTH_FILE" ]; then
      export XAUTHORITY="$XAUTH_FILE"
  fi
  # Cursor theme for X11 apps
  export XCURSOR_THEME=Helix-Invisible
  export XCURSOR_SIZE=48
  
  APP_PATH=/opt/onlyoffice/desktopeditors
  export LD_LIBRARY_PATH=$APP_PATH${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}
  exec $APP_PATH/DesktopEditors "$@"
  EOF
  chmod +x /usr/bin/desktopeditors
  ```

## Testing: 4K Resolution

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new session at 4K (3840x2160)
- [ ] Launch OnlyOffice via wrapper
- [ ] Verify full window renders (not just top quarter)
- [ ] Test window maximize/resize at 4K

## Testing: Cursor Theme

- [ ] Verify OnlyOffice uses Helix-Invisible cursor theme via XCURSOR_THEME
- [ ] Verify client-side cursor renders correctly over OnlyOffice
- [ ] Test cursor shape changes (text I-beam, resize handles)

## Design Doc Updates

- [ ] Update design.md with finding: OnlyOffice is Qt5+CEF, requires XWayland not native Wayland
- [ ] Document the correct environment variables needed