# Requirements: System / Light / Dark Three-Way Theme Toggle in Settings

## Background

The app currently derives its colour mode from the OS preference at load time and
mirrors OS transitions live. There is no way for the user to pin it to light or dark
independently of their OS, and there is no UI on the Settings page to control it at all.

## User Stories

**US-1** — As a user, I want to choose between "System", "Light", and "Dark" in Settings
so that I can lock the app to a specific colour scheme regardless of my OS setting.

**US-2** — As a user who picks "System", I want the app to follow OS dark/light transitions
live so that I don't have to revisit Settings every time my system schedule changes.

**US-3** — As a user who picks "Light" or "Dark", I want the app to stay in that mode across
page reloads so that my preference survives navigation and browser restarts.

**US-4** — As a logged-in user, I want my explicit preference (light / dark / system) synced
to my account so that it can be consumed by the settings-sync-daemon for GNOME/Zed mirroring.

## Acceptance Criteria

1. The General Settings panel contains a labelled three-way segmented control: **System · Light · Dark**.
2. Selecting "Light" forces `palette.mode = 'light'` regardless of OS; OS change events are ignored.
3. Selecting "Dark" forces `palette.mode = 'dark'` regardless of OS; OS change events are ignored.
4. Selecting "System" resumes live OS sync; the current OS preference takes effect immediately.
5. The preference is persisted in `localStorage` under the key `helixColorScheme` and restored on reload.
6. On first load (no stored preference) the behaviour is identical to today: OS preference wins, live sync active.
7. When a logged-in user changes the preference the resolved `color_scheme` (`light` or `dark`) is
   sent to `v1UsersMeColorSchemeUpdate` (same API call as today).
8. The toggle is visible to all users (anonymous and authenticated); persistence is local-only for
   anonymous users.
