# fix(settings-sync-daemon): make Helix↔GNOME↔Zed theme sync reliable

## Summary

Three bugs in the inner-desktop theme sync, all in `api/cmd/settings-sync-daemon/main.go`. Toggling light/dark in the Helix UI now updates the GNOME desktop and the Zed editor reliably, in both directions, repeatedly — and a user's manually-picked Zed theme is preserved.

## Bugs fixed

1. **Zed stuck on `One Light` after dark→light→dark.** `theme` was in `USER_PREFERENCE_FIELDS`, so `mergeSettings` always preserved the on-disk value. The first dark→light apply landed; any subsequent write through `mergeSettings` (including the polling tick) re-pinned `One Light` and silently overwrote the new `Ayu Dark` that `syncFromHelix` had just set.

2. **30 s GNOME flakiness when WS event was missed.** `checkHelixUpdates` (the polling fallback) didn't call `applyGNOMEColorScheme` — only `syncFromHelix` did. So when the WebSocket subscriber missed a `config_changed` event during a reconnect, GNOME stayed on the old theme indefinitely.

3. **Light-mode wallpaper changed to stock Ubuntu Quokka.** `applyGNOMEColorScheme` swapped to `Questing_Quokka_Full_Light_3840x2160.png` in light mode. Reverted to `helix-logo.png` in both modes.

## Changes

- Replaced the blanket `theme` protection with `HELIX_MANAGED_THEMES = {"One Light", "Ayu Dark"}` and a small `effectiveTheme(apiTheme) string` helper that returns the API value when on-disk is unset or in `HELIX_MANAGED_THEMES`, otherwise preserves the on-disk value (a user's manual Zed-UI choice).
- Both `syncFromHelix` and `checkHelixUpdates` now call `effectiveTheme` before assigning `theme`.
- `extractUserOverrides` filters out `theme` so the daemon-local decision can't be polluted via the API user-overrides replay loop.
- `checkHelixUpdates` now calls `d.applyGNOMEColorScheme(config.ColorScheme)` on every poll. Idempotent gsettings writes mean it's safe to run on every tick; load-bearing for missed-WS-event recovery.
- `applyGNOMEColorScheme` keeps `helix-logo.png` for both modes (still set on both `picture-uri` and `picture-uri-dark`).
- Defensive: lowered WS reconnect backoff 5 s → 1 s, and `runConfigEventLoop` now calls `syncFromHelix` once on every successful (re)connect to pick up state changes that happened during a disconnect.

## Test plan

- [ ] In the inner Helix, register/onboard, start a spec-task session, wait for the desktop.
- [ ] Tail daemon logs: `docker compose exec sandbox-nvidia docker logs -f <ubuntu-external-container> 2>&1 | grep -E "config event|config_changed|applied GNOME|Updated settings.json"`
- [ ] Toggle dark↔light several times; after each, confirm `gsettings get org.gnome.desktop.interface color-scheme` and `cat /home/retro/.config/zed/settings.json | jq '.theme'` flip in both directions, repeatedly.
- [ ] Confirm wallpaper is `helix-logo.png` in both modes (`gsettings get org.gnome.desktop.background picture-uri`).
- [ ] Pick a custom Zed theme (e.g. `Solarized Dark`); toggle Helix dark↔light; confirm GNOME flips but Zed's `.theme` stays `"Solarized Dark"`.
- [ ] Force-test the polling fallback: `docker compose restart api`, immediately toggle, wait ≤30 s, confirm both surfaces converge.

Refs spec task 001998.
