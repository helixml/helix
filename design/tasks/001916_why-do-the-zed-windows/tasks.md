# Implementation Tasks

- [ ] In `desktop/sway-config/startup-app.sh`, after the `assign [...]` block (~line 374) and before the `default_border` block (~line 381), append `for_window ... floating enable, resize set ... move position center` rules for `dev.zed.Zed-Dev`, `Zed`, `google-chrome`, `Google-chrome` (1600×900), and `kitty`, `ghostty`, `acp-log-viewer` (1100×700)
- [ ] Rebuild the sway-helix desktop image (`./stack build` or equivalent) and start a fresh session
- [ ] Verify Zed launches as a centred ~1600×900 floating window with visible borders, not full screen
- [ ] Verify Chrome and the terminal apps also launch floating at the configured sizes
- [ ] Verify `$mod+f` (fullscreen toggle) and `$mod+Shift+space` (floating ↔ tiling toggle) still work
- [ ] Stream the desktop in a small browser viewport (~1280×720) and confirm Zed is no longer clipped at the bottom
- [ ] Open a PR titled `Float Zed and key apps by default in Sway desktop` referencing task 001916
