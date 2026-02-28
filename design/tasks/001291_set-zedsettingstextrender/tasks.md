# Implementation Tasks

- [x] Add `"text_rendering_mode": "grayscale"` to `d.helixSettings` in `syncFromHelix()` function in `api/cmd/settings-sync-daemon/main.go`
- [x] Verify GNOME `dconf-settings.ini` already has `font-antialiasing='grayscale'` (confirmed at line 27 - no change needed)
- [ ] Test: Start a new session and verify `~/.config/zed/settings.json` contains the setting
- [ ] Test: Confirm text in Zed renders with grayscale antialiasing (no color fringing on text edges)

## Notes

- GNOME settings already configured correctly in `desktop/ubuntu-config/dconf-settings.ini` (`font-antialiasing='grayscale'`)
- No rebuild needed for settings-sync-daemon changes (Air hot-reloads Go changes); just start a new session to pick up changes
- Testing requires deploying to a running stack and starting a new session - marking as user verification