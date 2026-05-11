# Implementation Tasks

- [ ] In `api/cmd/settings-sync-daemon/main.go` `checkHelixUpdates()`, call `d.applyGNOMEColorScheme(config.ColorScheme)` on every poll so the 30 s polling fallback actually repairs GNOME state when a WebSocket `config_changed` event was missed.
- [ ] In `api/cmd/settings-sync-daemon/main.go` `applyGNOMEColorScheme()`, remove the light-mode wallpaper override (`Questing_Quokka_Full_Light_3840x2160.png`) — keep `file:///usr/share/backgrounds/helix-logo.png` for both light and dark. Update the leading comment to match.
- [ ] (Optional) Lower the WS reconnect backoff in `subscribeConfigEvents` from 5 s to 1 s, and call `syncFromHelix()` once on every successful WS reconnect (before entering the read loop) so changes that happened during a disconnect are picked up immediately rather than waiting for the next poll.
- [ ] Rebuild the desktop image: `./stack build-ubuntu`. Start a **new** spec-task session in the inner Helix (settings-sync-daemon does not hot-reload).
- [ ] Verify in the inner Helix at `http://localhost:8080`: toggle light↔dark several times in the top bar, confirm GNOME follows within ~1 s on the WS path and within ≤30 s on the polling path (force-test by `docker compose restart api`, toggle, wait). Confirm wallpaper stays `helix-logo.png` in both modes.
- [ ] Tail `docker logs <ubuntu-external-container>` during the test for `applied GNOME color-scheme=…` lines to confirm both code paths fire as expected.
