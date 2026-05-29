# Implementation Tasks: Fix Light/Dark Theme Toggle Not Returning to Dark Mode

- [ ] In `crates/theme_settings/src/settings.rs:292-315`, replace the hardcoded `ThemeAppearanceMode::System` in the `Static(_)` arm of `set_mode()` with the `mode` parameter passed to the function.
- [ ] In the same arm, preserve the previous static theme name by routing it into `light` or `dark` based on `Theme::appearance()`, defaulting only the opposite slot from `DEFAULT_LIGHT_THEME` / `DEFAULT_DARK_THEME`.
- [ ] Handle the case where the previous static theme name cannot be resolved in `ThemeRegistry` (extension removed): fall back to defaulting both slots.
- [ ] Audit `set_theme()` at `crates/theme_settings/src/settings.rs:233-264` for the same Static→Dynamic conversion bug and apply a symmetric fix if present.
- [ ] Check `IconThemeSelection::set_mode` for the same hardcoded `System` pattern and apply the equivalent fix if it exists.
- [ ] Extend `test_toggle_theme_mode_persists_and_updates_active_theme` (`crates/workspace/src/workspace.rs:15696-15758`) to assert that starting from `Static("X")` + `ToggleMode(Dark)` yields `Dynamic { mode: Dark, dark: "X", light: DEFAULT_LIGHT }`.
- [ ] Add a regression test that cycles Light → Dark → Light from a `Static` starting state and asserts the active theme name at each step.
- [ ] Verify the `cx.observe_window_appearance()` callback in `crates/workspace/src/workspace.rs:1759-1766` does not override an explicit `Light`/`Dark` mode after the fix.
- [ ] Manually QA: from a clean settings.json with no `theme` key, toggle to Light, toggle to Dark, restart Zed, confirm dark mode persists; repeat starting from a `Static("Solarized Light")` setting.
- [ ] Update CHANGELOG / release notes entry noting the fixed toggle behaviour.
