# Implementation Tasks

## Investigation

- [ ] Start session at 4K resolution (3840x2160)
- [ ] Launch OnlyOffice and confirm only top quarter renders
- [ ] Screenshot the broken 4K rendering for documentation
- [ ] Verify it works correctly at 1080p (baseline)

## Fix: Wrapper Script

- [ ] Add wrapper script to `Dockerfile.ubuntu-helix` after OnlyOffice install (~line 365):
  ```bash
  mv /usr/bin/desktopeditors /usr/bin/desktopeditors.real
  cat > /usr/bin/desktopeditors << 'EOF'
  #!/bin/bash
  export XCURSOR_THEME=Helix-Invisible
  export XCURSOR_SIZE=48
  export GTK_CURSOR_THEME_NAME=Helix-Invisible
  exec /usr/bin/desktopeditors.real \
      --force-device-scale-factor=1 \
      --high-dpi-support=1 \
      --ozone-platform=wayland \
      --enable-features=UseOzonePlatform \
      "$@"
  EOF
  chmod +x /usr/bin/desktopeditors
  ```

## Testing: 4K Resolution

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new session at 4K (3840x2160)
- [ ] Launch OnlyOffice
- [ ] Verify full window renders (not just top quarter)
- [ ] Test window maximize/resize at 4K

## Testing: Cursor Theme

- [ ] Verify OnlyOffice cursor is invisible (using Helix-Invisible)
- [ ] Verify client-side cursor renders correctly over OnlyOffice
- [ ] Test cursor shape changes (text I-beam, resize handles)

## Alternative Flags (if scale factor=1 doesn't work)

- [ ] Try `--force-device-scale-factor=2` for 4K
- [ ] Try without `--high-dpi-support=1`
- [ ] Try `--enable-features=WaylandWindowDecorations`

## Cursor Fallback (if theme not respected)

- [ ] Check if `--gtk-version=4` helps with cursor theme
- [ ] Try `GDK_BACKEND=wayland` in wrapper
- [ ] Accept dual cursors as known limitation if nothing works