# Requirements: Fix Light/Dark Theme Toggle Not Returning to Dark Mode

## Background

In Zed, the `theme::ToggleMode` action (typically bound to a keybinding or invoked from the command palette) should cycle the editor appearance between Light and Dark. Users report that toggling to Light and then back to Dark does not actually return them to a dark theme — the editor either stays light or jumps to an unexpected theme. The user's previously chosen theme is also silently replaced with a built-in default.

## User Stories

**US-1: Toggle back to dark**
As a Zed user who has switched to Light mode, I want to toggle back to Dark mode and immediately see a dark theme applied, so that I can comfortably alternate between light and dark while working.

**US-2: Preserve my chosen theme**
As a Zed user who has manually picked a non-default theme (e.g. "Solarized Light"), I want toggling the appearance mode to keep my chosen theme in its corresponding slot rather than reverting it to a built-in default.

**US-3: Persisted mode survives restart**
As a Zed user, I want my explicit Light or Dark choice to be persisted in `settings.json` exactly as I selected it (not silently rewritten to "System"), so that the same mode is restored next time I launch Zed.

## Acceptance Criteria

- **AC-1:** Starting from any theme configuration, invoking `theme::ToggleMode` to switch to Light then invoking it again must result in a dark theme being active in the editor.
- **AC-2:** After toggling from a `Static("X")` theme to Dark mode, the persisted setting must store `mode: Dark`, the dark slot must be populated with a dark theme, and re-toggling must continue to alternate Light ↔ Dark deterministically.
- **AC-3:** Toggling appearance mode from a `Static` theme must NOT silently set `mode` to `System`. The mode actually requested by the toggle handler must be the mode persisted.
- **AC-4:** A user's previously-active theme name must be preserved into the slot matching its appearance (a light theme into `light`, a dark theme into `dark`) when converting from `Static` to `Dynamic`.
- **AC-5:** Existing test `test_toggle_theme_mode_persists_and_updates_active_theme` (`crates/workspace/src/workspace.rs:15696`) continues to pass, and is extended (or supplemented) with a regression test that explicitly verifies the Light → Dark → Light cycle from a `Static` starting state.
- **AC-6:** Settings already on disk in the current (buggy) `Dynamic` shape continue to load without error; the fix is forward-only and requires no migration script.
- **AC-7:** Equivalent behaviour holds for `IconThemeSelection::set_mode` if that code path exhibits the same defect — fix together to avoid divergent logic.

## Out of Scope

- Redesigning the theme picker UI.
- Changing the System-appearance observer behaviour for users who explicitly choose `mode: System`.
- Adding new theme settings, new actions, or new keybindings.
