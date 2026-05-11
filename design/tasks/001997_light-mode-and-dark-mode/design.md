# Design: Theme Follows OS System Settings

## Current Architecture (as of 2026-05)

Helix already has most of the plumbing for OS-aware theming. It just doesn't expose a way for users to opt back into it.

```
Browser (React + MUI)
  └─ contexts/theme.tsx
       ├─ getInitialMode(): localStorage → prefers-color-scheme → 'dark'
       ├─ matchMedia('(prefers-color-scheme: light)').addEventListener('change', …)
       │     └─ only fires if localStorage is empty
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
- `frontend/src/contexts/theme.tsx:9-42` — initial detection + media-query listener
- `frontend/src/components/system/Page.tsx:79,342` — toggle button in topbar
- `api/pkg/server/user_handlers.go:403-485` — color-scheme PUT endpoint + WS publish
- `api/pkg/server/zed_config_handlers.go:297-309` — maps color scheme to Zed theme name
- `api/cmd/settings-sync-daemon/main.go:908-931` — applies GNOME/GTK theme

## Root Cause

1. `theme.tsx` treats the presence of a `localStorage.themeMode` value as "user has opted out of OS following — never follow OS again." There is no UI control to clear that flag.
2. The toggle is a binary Light↔Dark switch with no third "System" state.
3. As soon as the user clicks the toggle even once, OS sync stops permanently for that browser.

The detection logic itself works; the UX simply has no way back to "follow system."

## Decision

Introduce an explicit three-state mode: **Light / Dark / System**, with **System as the default** for users who have not made a choice. Persist the *user's mode selection* (`light` | `dark` | `system`) rather than the *resolved color* (`light` | `dark`).

### Why three states, not "auto-follow when localStorage empty"
- Discoverability: users today have no signal that OS-following exists, and no way to re-enable it after toggling.
- Matches widely understood patterns (VS Code, GitHub, macOS apps).
- Eliminates the "ghost localStorage value" foot-gun where a stray click silently disables OS-following forever.

### Why store the *mode* not the resolved color
- If we store the resolved color (`light`/`dark`) on the server, we can't represent "this user is in System mode" — when they sign in on a second device, that device can't tell whether to follow its OS or honor a fixed pin.
- Storing `system` server-side lets each device resolve to its own OS preference independently. The server still gets a resolved value sent for inner-desktop syncing (see below).

## Frontend changes (`frontend/`)

### `frontend/src/contexts/theme.tsx`
- Replace the binary `mode` state with a `themePreference: 'light' | 'dark' | 'system'` and a derived `resolvedMode: 'light' | 'dark'`.
- `getInitialPreference()`:
  1. Read `localStorage.themePreference` if present and valid.
  2. Otherwise default to `'system'` (do **not** pre-resolve to dark).
- `resolvedMode` computation:
  - If `themePreference === 'system'`: read `window.matchMedia('(prefers-color-scheme: light)').matches`.
  - Otherwise: `themePreference`.
- The `prefers-color-scheme` `change` listener fires whenever it fires, but only updates `resolvedMode` (and POSTs to the API) when `themePreference === 'system'`.
- On any change to `resolvedMode`, POST `{color_scheme: resolvedMode}` to `/api/v1/users/me/color-scheme` so the inner desktop stays in sync. This already happens; just make sure it fires for both manual toggles and system-driven changes.
- One-time migration: if `localStorage.themeMode` exists (the legacy key), copy its value to `themePreference` and delete the old key. Users who never toggled (no legacy key) land on `system`.

### `frontend/src/components/system/Page.tsx` (and any other place the toggle lives)
- Replace the binary toggle icon with a small menu/dropdown showing **Light / Dark / System** with the current selection indicated.
- Acceptable lighter-weight alternative: keep an icon button, but cycle Light → Dark → System on click and show the current mode in the tooltip and via the icon (sun / moon / desktop). The dropdown is preferred for discoverability — pick whichever is more idiomatic for the existing topbar.

### `frontend/src/hooks/useLightTheme.tsx`
- Should consume `resolvedMode`, not `themePreference`. No callers outside the context need to know about System mode.

## Backend changes (`api/`)

### `api/pkg/server/user_handlers.go`
- The `PUT /api/v1/users/me/color-scheme` endpoint currently accepts `"light" | "dark" | ""`. Continue accepting only resolved values (`"light" | "dark"`) — the server doesn't need to know about `"system"` because the browser always sends the resolved color whenever it changes.
- No schema change to `UserMeta.Config.ColorScheme`. It continues to hold the last *resolved* color the user is seeing, which is what the inner desktop needs.

### `api/pkg/server/zed_config_handlers.go`
- No change. It already maps the stored color scheme to a Zed theme name.

### `api/cmd/settings-sync-daemon/main.go`
- No change. It already reacts to `config_changed` WS events.

## Edge cases

- **OS doesn't expose preference** (older browsers, headless): `matchMedia('(prefers-color-scheme: light)').matches` returns `false` and no `dark` query matches either. Treat unknown as dark (current behavior); document it.
- **User in System mode signs in on a brand-new device**: no server-side pin, browser resolves locally from OS. First POST after load syncs the inner desktop.
- **User in System mode is mid-session when OS flips**: media-query listener fires → state updates → MUI re-themes → POST to API → WS event → settings-sync-daemon re-runs `gsettings` and Zed theme. Already works for the OS-detection-on-first-load path; we're extending the same path to explicit System mode.
- **Two browser tabs open**: each tab independently resolves and may both POST. The API call is idempotent and the WS broadcast handles convergence.

## Notes for future agents

- The detection and propagation pipeline is already complete and works. The bug is purely UX: there is no way to opt back into OS-following after a single manual toggle. Don't reinvent the API/WS/daemon path — extend the existing one.
- The legacy `localStorage` key is `themeMode` (see `theme.tsx:9` constant `THEME_MODE_KEY`). Pick a new key like `themePreference` so the migration check is unambiguous.
- The settings-sync-daemon authoritative source is `UserMeta.Config.ColorScheme` (string `"light"` / `"dark"` / `""`). Keep storing resolved colors there — daemons inside the container have no way to query the user's OS.
