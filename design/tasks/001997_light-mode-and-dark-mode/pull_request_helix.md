# fix(frontend): make theme follow OS appearance changes

## Summary

The Helix UI didn't reliably follow the OS-level light/dark setting (e.g., macOS System Settings → Appearance). The detection code was in place — `prefers-color-scheme` was read on initial load and a `change` listener was registered — but as soon as a user clicked the in-app sun/moon toggle once, a `localStorage.themeMode` value was written and the listener silently early-returned forever after, with no UI to clear it.

The fix removes the lockout entirely. The model is now **the most recent change wins**, regardless of source: an OS transition fires the media-query listener, a manual toggle flips local state, both push the resolved color to `/api/v1/users/me/color-scheme` so the inner GNOME desktop and Zed editor mirror the change via the existing `settings-sync-daemon` path. There is no persisted preference — every page load re-resolves from the OS.

## Changes

- `frontend/src/contexts/theme.tsx`: drop the `THEME_MODE_KEY` constant, the localStorage read in `getInitialMode()`, the localStorage gate in the media-query `change` listener, and the localStorage write in `toggleMode()`. Add a one-time cleanup that removes any leftover legacy `themeMode` key on load.
- `frontend/index.html`: drop the `localStorage.themeMode` read from the pre-mount no-flash script; resolve directly from `prefers-color-scheme` instead.

## Verification

End-to-end in the inner Helix at `localhost:8080` using Chrome DevTools `emulate.colorScheme`:

| Step | Expected | Result |
|---|---|---|
| First load with OS=light, empty localStorage | body bg white | ✅ `rgb(255,255,255)` |
| Seed legacy `themeMode=dark`, reload | key removed, body matches OS=light | ✅ key absent, body white |
| App in dark, emulate OS → light | body live-updates to light, no reload | ✅ |
| Click sun/moon toggle | body flips, no localStorage write | ✅ `themeMode` stays absent |
| After manual toggle, emulate OS → dark | body re-syncs to OS dark (last-change-wins) | ✅ `rgb(18,18,20)` |

Inner-desktop sync (GNOME color-scheme, GTK theme, wallpaper, Zed theme) still goes through the existing `v1UsersMeColorSchemeUpdate` → WebSocket → `settings-sync-daemon` path; it fires for both manual toggles and OS transitions. Verifying on a real macOS host is a manual check for the reviewer — the headless dev environment has no OS appearance to flip.

## Screenshots

![OS dark, app dark](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001997_light-mode-and-dark-mode/screenshots/01-os-dark-app-dark.png)

![OS light, app light](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001997_light-mode-and-dark-mode/screenshots/02-os-light-app-light.png)
