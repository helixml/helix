# Implementation Tasks

- [x] In `api/cmd/settings-sync-daemon/main.go`, empty `USER_PREFERENCE_FIELDS` (kept the symbol with a comment explaining the change) so `mergeSettings` no longer pins the on-disk Zed theme value.
- [x] In `api/cmd/settings-sync-daemon/main.go`, added `HELIX_MANAGED_THEMES = {"One Light", "Ayu Dark"}` and `effectiveTheme(apiTheme string) string` helper that reads the on-disk `theme` and returns `apiTheme` only when on-disk is unset or in `HELIX_MANAGED_THEMES`. Otherwise returns the on-disk value.
- [x] `syncFromHelix` now uses `if t := d.effectiveTheme(config.Theme); t != "" { d.helixSettings["theme"] = t }`.
- [x] `checkHelixUpdates()` mirrors the same `effectiveTheme`-guarded assignment to `newHelixSettings`.
- [x] Also added: `extractUserOverrides` now skips `"theme"` so the daemon-local decision doesn't leak into the user-overrides → API replay loop (would otherwise create a stale snapshot that replays back on next sync).
- [x] `checkHelixUpdates()` now calls `d.applyGNOMEColorScheme(config.ColorScheme)` on every poll — same call syncFromHelix makes. Idempotent and load-bearing for missed-WS-event recovery.
- [x] `applyGNOMEColorScheme()` no longer references `Questing_Quokka_Full_Light_3840x2160.png`. Both light and dark use `helix-logo.png` (still set on both `picture-uri` and `picture-uri-dark`). Comment updated.
- [x] (Optional) Lowered WS reconnect backoff in `subscribeConfigEvents` from 5 s → 1 s. `runConfigEventLoop` now also calls `syncFromHelix()` once on every successful (re)connect before entering the read loop — picks up state changes that happened during a disconnect without waiting for the next poll.
- [~] Rebuild the desktop image (`./stack build-ubuntu`) and start a **new** spec-task session in the inner Helix — settings-sync-daemon does not hot-reload.
- [ ] Verify in the inner Helix at `http://localhost:8080`: register/onboard, start a session, then toggle dark↔light multiple times in the top bar. After each toggle, inspect both surfaces inside the desktop container: `gsettings get org.gnome.desktop.interface color-scheme` and `cat /home/retro/.config/zed/settings.json | jq '.theme'`. Confirm both flip in both directions, repeatedly. Confirm wallpaper is `helix-logo.png` in both modes.
- [ ] Verify the custom-theme preservation: with the session running, manually edit `~/.config/zed/settings.json` (or pick via Zed's UI) to set `theme` to something outside the managed set (e.g. `"Solarized Dark"`). Toggle Helix dark↔light. Confirm GNOME flips but `~/.config/zed/settings.json` `.theme` stays `"Solarized Dark"`.
- [ ] Force-test the polling fallback: with a session running, `docker compose restart api`, immediately toggle, wait ≤30 s, confirm both GNOME and Zed converge to the new theme.
- [ ] Tail `docker logs <ubuntu-external-container>` during the test for `applied GNOME color-scheme=…` and `Updated settings.json` lines to confirm both code paths fire.
