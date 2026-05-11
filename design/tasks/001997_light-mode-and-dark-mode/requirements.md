# Requirements: Theme Follows OS System Settings

## Problem

Light/dark mode in the Helix UI does not reliably follow the OS-level appearance setting (e.g., macOS System Settings → Appearance). Once a user has interacted with the in-app theme toggle, their choice is stored in `localStorage` and the app stops following the OS — there is no way to opt back into "follow system" behavior short of clearing browser storage. The inner desktop (GNOME + Zed running in the Ubuntu container) inherits whatever the browser sends, so it has the same gap.

## User Stories

### US1 — Follow OS by default
As a new user on macOS (or any OS with light/dark switching), I want the Helix UI to match my OS appearance the first time I open it, so it visually fits the rest of my desktop without any setup.

**Acceptance criteria:**
- A first-time user (no stored preference) sees light mode when their OS is in light mode and dark mode when their OS is in dark mode.
- Switching the OS appearance while the Helix tab is open updates the UI within ~1 second, no reload required.

### US2 — Explicit "System" option
As a user who has previously toggled the theme manually, I want to opt back into "follow system" without clearing browser storage.

**Acceptance criteria:**
- The theme control offers three explicit choices: **Light**, **Dark**, **System**.
- Selecting **System** removes the locked-in preference and the UI immediately matches the current OS setting.
- The currently selected mode (Light/Dark/System) is visible from the control.

### US3 — Inner desktop follows the same theme
As a user, I want the inner desktop (GNOME, Zed, wallpaper) to match whatever theme the Helix UI is showing, including when that theme is being driven by the OS.

**Acceptance criteria:**
- When the browser theme changes — whether from a manual toggle or from an OS change in System mode — the inner desktop's GNOME color-scheme, GTK theme, wallpaper, and Zed editor theme update to match.
- A user in **System** mode who switches their OS from light to dark sees the inner desktop change without taking any action in Helix.

### US4 — Preference persists across sessions and devices
As a user who has explicitly chosen Light or Dark, I want that choice to stick across reloads and across devices where I sign in.

**Acceptance criteria:**
- An explicit Light or Dark choice survives a page reload and a sign-out/sign-in.
- An explicit choice on one browser is reflected when the same user signs in on another browser (because it's persisted to `UserMeta.Config.ColorScheme` server-side).
- A **System** choice does not pin a server-side value to a fixed light/dark — the server reflects whatever the browser is currently showing based on the OS.

## Out of Scope

- Adding additional themes beyond the existing light and dark palettes.
- Detecting the OS appearance from inside the Ubuntu container directly (the browser remains the source of truth for what the user is seeing).
- Per-session or per-project theme overrides.
