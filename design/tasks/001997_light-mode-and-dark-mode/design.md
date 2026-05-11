# Design: Theme Follows OS System Settings

## Current Architecture (as of 2026-05)

Helix already has most of the plumbing for OS-aware theming. The bug is in one specific gating condition.

```
Browser (React + MUI)
  └─ contexts/theme.tsx
       ├─ getInitialMode(): localStorage → prefers-color-scheme → 'dark'
       ├─ matchMedia('(prefers-color-scheme: light)').addEventListener('change', …)
       │     └─ ❌ only fires if localStorage is empty  ← THE BUG
       └─ toggle button: writes localStorage, locks the choice
                ↓ POST /api/v1/users/me/color-scheme
                ↓
API (api/pkg/server/user_handlers.go)
  ├─ stores in UserMeta.Config.ColorScheme
  └─ publishes config_changed event over user WebSocket
                ↓
settings-sync-daemon (inside Ubuntu container)
  ├─ receives event, fetches /api/v1/sessions/{id}/zed-config
  └─ applies via gsettings (color-scheme, gtk-theme, wallpaper) + Zed theme
```

Relevant files:
- `frontend/src/contexts/theme.tsx:9-42` — initial detection + media-query listener (the listener has the buggy `if (localStorage.getItem(THEME_MODE_KEY)) return` early-out)
- `frontend/src/components/system/Page.tsx:79,342` — sun/moon toggle button in topbar
- `api/pkg/server/user_handlers.go:403-485` — color-scheme PUT endpoint + WS publish
- `api/pkg/server/zed_config_handlers.go:297-309` — maps color scheme to Zed theme name
- `api/cmd/settings-sync-daemon/main.go:908-931` — applies GNOME/GTK theme

## Root Cause

`theme.tsx` treats the presence of a `localStorage.themeMode` value as "user has opted out of OS following — never follow OS again." There is no UI control to clear that flag, so a single click of the toggle permanently disables OS sync for that browser.

## Decision

**The most recent change wins, regardless of source.** Both the user (via the sun/moon toggle) and the OS (via a `prefers-color-scheme` transition) emit change events; whichever fires last sets the current mode. Neither is privileged over the other.

Concretely:
- Keep the existing binary sun/moon toggle. No new icon, no third state.
- Drop the `localStorage` lock-out entirely. The media-query `change` listener always updates the resolved mode and POSTs to the API.
- The user's manual click flips the current mode and POSTs to the API, but does not pin a "user preference" that would block a later OS transition.

### Why this over the previous "three-state" proposal
- Avoids the icon/UX problem of representing a "System" mode in a single button.
- Matches the user's stated mental model: "both the system and the user can both change the current setting."
- Simpler implementation: removes one whole state dimension and the migration that came with it.

### Detection is event-driven, not polled
The browser's `matchMedia(...).addEventListener('change', …)` only fires on an actual OS-level *transition*. So if the user manually toggles to a mode different from the OS and the OS state never changes after that, the manual choice persists indefinitely (until another click or a reload, which re-resolves to the OS). This is consistent with "last change wins" — there simply isn't a later change to override it.

### Why we don't need to persist the user's manual choice
- An explicit toggle changes the current visual immediately and POSTs the resolved color to the server, so the inner desktop follows.
- If the OS later changes, the user almost certainly wants the app to follow — the OS change is the freshest signal of user intent.
- On reload, we read `prefers-color-scheme` again and resolve to whatever the OS currently says, which is the same answer the OS-change handler would have produced.

## Frontend changes (`frontend/`)

### `frontend/src/contexts/theme.tsx`
- `getInitialMode()` simplifies to: read `window.matchMedia('(prefers-color-scheme: light)').matches`, return `'light'` or `'dark'`. No localStorage read.
- The `prefers-color-scheme` `change` listener:
  - Remove the `if (localStorage.getItem(THEME_MODE_KEY)) return` early-out.
  - Always update `mode` to match the new OS value.
  - Always POST the new value to `/api/v1/users/me/color-scheme` (it already does this; just remove the gate).
- The `toggleMode()` function:
  - Remove the `localStorage.setItem(THEME_MODE_KEY, next)` line.
  - Keep flipping `mode` and POSTing to the API.
- Remove the `THEME_MODE_KEY` constant and any reads/writes once nothing references them.
- One-time cleanup on load: if `localStorage.themeMode` exists from prior versions, delete it. (One line, no real migration logic — we no longer care what it said.)

### `frontend/src/components/system/Page.tsx`
- No change. The existing sun/moon toggle button keeps working as-is.

### `frontend/src/hooks/useLightTheme.tsx`
- No change. Still consumes the resolved `mode`.

## Backend changes (`api/`)

None. The PUT endpoint, the WebSocket publish, and the settings-sync-daemon all stay as they are.

## Edge cases

- **OS doesn't expose preference** (older browsers, headless): `matchMedia('(prefers-color-scheme: light)').matches` returns `false` and `(prefers-color-scheme: dark)` also returns false. Treat unknown as dark (current default). The change listener will simply never fire; the manual toggle still works.
- **User toggles, then OS flips to the same mode the user already picked**: no visible change, listener fires, POST is a no-op repeat. Fine.
- **Two browser tabs open**: each tab independently resolves and POSTs on OS change. The API call is idempotent and the WS broadcast handles convergence to the inner desktop.
- **User manually toggles to light, OS later flips to light too**: app stays light, listener fires with `light`, no visible change, POST is a repeat. Fine.
- **User manually toggles to light while OS is dark, then reloads**: page reads `prefers-color-scheme` → dark on reload. The manual override does not survive reload. This is acceptable per the "OS-always-wins" model and avoids needing persistence logic.

## Implementation notes

- **Discovery during implementation:** there is a second piece of code that reads `localStorage.themeMode` — an inline pre-mount `<script>` in `frontend/index.html` that sets `document.documentElement.style.{backgroundColor,color}` before React mounts so the page doesn't flash dark for users who prefer light. It had to be updated in the same commit (drop the localStorage read, resolve directly from `prefers-color-scheme`); otherwise the html element would still flash a stale color from the legacy localStorage value on first render after this change ships.
- **Verified end-to-end** in the inner Helix at `localhost:8080` with chrome-devtools `emulate.colorScheme`:
  - Initial load with OS=light → body bg `#ffffff`, no `themeMode` localStorage key.
  - Pre-seed `localStorage.themeMode = 'dark'`, reload → key is removed by the cleanup, body resolves to OS (`#ffffff`).
  - With app showing dark, emulate OS → light → body live-updates to `#ffffff` with no reload.
  - Click sun/moon toggle → body flips, `localStorage.themeMode` stays absent.
  - After manual toggle, emulate OS → dark → body flips back to `#121214`. Confirms "last change wins" — the manual toggle no longer locks out future OS events.
- **Total diff:** 13 lines in `frontend/src/contexts/theme.tsx` and 7 lines in `frontend/index.html`. No backend, daemon, or `Page.tsx` changes — the existing sun/moon icon and the existing API/WS pipeline already do the right thing once the localStorage gate is removed.
- **Inner-desktop verification:** the API call `v1UsersMeColorSchemeUpdate` still fires on every mode change (manual or OS), so the existing `settings-sync-daemon` path into GNOME/Zed is unchanged. Verifying it on a real macOS host is a manual step for the user — the dev container doesn't have an OS appearance to flip.

## Notes for future agents

- The detection and propagation pipeline is already complete and works. The bug is one gating condition (`if (localStorage.getItem(THEME_MODE_KEY)) return`) plus the matching `setItem` in the toggle handler. Removing both, and removing the localStorage read in `getInitialMode`, is the entire fix.
- The settings-sync-daemon authoritative source is `UserMeta.Config.ColorScheme` (string `"light"` / `"dark"` / `""`). Keep storing resolved colors there — daemons inside the container have no way to query the user's OS, so the browser must keep telling them.
- Resist the urge to add a "follow system" toggle. The whole point of this design is that following the system is the implicit, always-on behavior; the toggle is a temporary override.
