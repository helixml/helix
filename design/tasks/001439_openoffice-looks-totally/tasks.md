# Implementation Tasks

## Investigation

- [ ] Launch OnlyOffice in AMD64 session to reproduce issue
- [ ] Check if OnlyOffice is using X11 or Wayland: `cat /proc/$(pgrep -f desktopeditors)/environ | tr '\0' '\n' | grep -E 'DISPLAY|WAYLAND'`
- [ ] Screenshot the broken rendering for documentation
- [ ] Check OnlyOffice's Electron version: `strings /opt/onlyoffice/desktopeditors/DesktopEditors | grep -i electron`

## Fix: Wrapper Script

- [ ] Create wrapper script in Dockerfile.ubuntu-helix after OnlyOffice install:
  ```bash
  mv /usr/bin/desktopeditors /usr/bin/desktopeditors.real
  printf '#!/bin/bash\nexec /usr/bin/desktopeditors.real --ozone-platform=wayland --enable-features=UseOzonePlatform,WaylandWindowDecorations "$@"\n' > /usr/bin/desktopeditors
  chmod +x /usr/bin/desktopeditors
  ```
- [ ] Patch .desktop file to include same flags
- [ ] Apply same fix to Dockerfile.sway-helix if needed

## Alternative Flags to Try (if above doesn't work)

- [ ] Try `ELECTRON_OZONE_PLATFORM_HINT=wayland` environment variable
- [ ] Try `--disable-gpu-sandbox` flag
- [ ] Try `--use-gl=egl` for GPU rendering
- [ ] Try `--disable-gpu` as last resort (software rendering)

## Testing

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new AMD64 session
- [ ] Launch OnlyOffice and verify full window renders
- [ ] Test window resize
- [ ] Test opening a .docx file
- [ ] Test opening a .xlsx file
- [ ] Document cursor behavior (may still have dual cursors - this is expected)
- [ ] Screenshot working state

## Fallback (if Electron flags don't work)

- [ ] Consider adding XWayland to GNOME headless for OnlyOffice specifically
- [ ] Check if newer OnlyOffice .deb has better Wayland support
- [ ] File upstream issue with OnlyOffice if needed