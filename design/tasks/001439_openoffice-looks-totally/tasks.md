# Implementation Tasks

## Investigation

- [ ] SSH into ARM64 VM and launch LibreOffice manually to reproduce issue
- [ ] Check current VCL backend: `libreoffice --version` and environment inspection
- [ ] Verify `GDK_BACKEND=wayland` is set when LibreOffice launches
- [ ] Check if `libreoffice-gtk4` package is installed in ARM64 image
- [ ] Screenshot the broken rendering for documentation

## Fix: Environment Variables

- [ ] Add `SAL_USE_VCLPLUGIN=gtk4` to Dockerfile.ubuntu-helix ENV block
- [ ] Add `SAL_DISABLE_CURSOR_ANIMATION=1` to same ENV block
- [ ] Export same variables in `desktop/ubuntu-config/startup-app.sh`

## Fix: Package Dependencies (if needed)

- [ ] Check if `libreoffice-gtk4` exists in Ubuntu 25.10 repos
- [ ] Add package to Dockerfile.ubuntu-helix ARM64 LibreOffice install section
- [ ] If gtk4 unavailable, try `gtk3` backend instead

## Testing

- [ ] Rebuild image: `./stack build-ubuntu`
- [ ] Start new ARM64 session
- [ ] Launch LibreOffice Writer and verify full window renders
- [ ] Test window resize
- [ ] Test menu opening
- [ ] Verify cursor behavior (document any expected dual-cursor situation)
- [ ] Screenshot working state

## Fallback (if VCL fix doesn't work)

- [ ] Try forcing specific geometry: `SAL_WINDOW_SIZE=1920x1080`
- [ ] Consider adding XWayland for LibreOffice specifically
- [ ] Test alternative: `GDK_BACKEND=x11` just for LibreOffice (requires Xwayland)