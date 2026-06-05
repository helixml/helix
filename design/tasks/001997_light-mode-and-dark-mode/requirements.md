# Requirements: Theme Follows OS System Settings

## Problem

Light/dark mode in the Helix UI does not reliably follow the OS-level appearance setting (e.g., macOS System Settings → Appearance). Once a user has interacted with the in-app theme toggle, their choice is stored in `localStorage` and the app stops following the OS — there is no way to opt back into "follow system" behavior short of clearing browser storage. The inner desktop (GNOME + Zed running in the Ubuntu container) inherits whatever the browser sends, so it has the same gap.

## Approach

Keep the existing binary sun/moon toggle — no new "System" icon. The model is simply: **the most recent change wins, whether it came from the user or the OS**. The user can flip the toggle whenever they want; if the OS appearance later transitions, that transition becomes the most recent change and re-syncs the app. There is no permanent "user has opted out of OS following" state.

Note: OS-driven updates fire only on an actual OS-level *transition* (the browser's `prefers-color-scheme` media-query `change` event). If the OS state never changes, the user's manual choice stays in effect until they click again or reload.

## User Stories

### US1 — Follow OS by default
As a new user on macOS (or any OS with light/dark switching), I want the Helix UI to match my OS appearance the first time I open it, so it visually fits the rest of my desktop without any setup.

**Acceptance criteria:**
- A first-time user (no stored preference) sees light mode when their OS is in light mode and dark mode when their OS is in dark mode.

### US2 — OS transitions propagate even after a manual toggle
As a user, I want OS-level appearance transitions to propagate into Helix even if I have previously clicked the in-app toggle.

**Acceptance criteria:**
- Switching the OS appearance while the Helix tab is open updates the UI within ~1 second, no reload required.
- This holds regardless of whether I have ever clicked the in-app toggle. There is no "locked" state that prevents OS sync.

### US3 — Manual toggle is the most recent change until something else changes
As a user, I want to be able to flip the toggle and have the UI change immediately, even if my OS is set to the opposite mode.

**Acceptance criteria:**
- Clicking the sun/moon toggle changes the UI to the opposite mode immediately.
- That manual choice holds until the next change event, which is either (a) I click the toggle again or (b) the OS appearance transitions.

### US4 — Inner desktop follows the same theme
As a user, I want the inner desktop (GNOME, Zed, wallpaper) to match whatever theme the Helix UI is showing, including when that theme is being driven by the OS.

**Acceptance criteria:**
- When the browser theme changes — whether from a manual toggle or from an OS change — the inner desktop's GNOME color-scheme, GTK theme, wallpaper, and Zed editor theme update to match within a few seconds.
- A user who flips their OS from light to dark sees the inner desktop change without taking any action in Helix.

## Out of Scope

- Adding a third explicit "System" option to the toggle (rejected: no clean icon, and the OS-always-wins model removes the need for one).
- Adding additional themes beyond the existing light and dark palettes.
- Detecting the OS appearance from inside the Ubuntu container directly (the browser remains the source of truth for what the user is seeing).
- Per-session or per-project theme overrides.
