# Implementation Tasks: Fix Zed Theme Not Reverting on OS Dark/Light Mode Toggle

## Reproduce & localize
- [ ] In the full Zed repo, reproduce: set theme mode = `System`, toggle OS appearance light→dark→light, confirm it sticks on dark.
- [ ] Add temporary `log::info!` at the four chain points: `Window::appearance_changed` (gpui/src/window.rs:2274), the workspace `observe_window_appearance` closure (workspace.rs:1781), `configured_theme` after computing `theme_name` (theme_settings.rs:155), and `GlobalTheme::update_theme` (theme.rs:329).
- [ ] Toggle light→dark→light and read the logs; identify the exact step where the reverse transition diverges (callback not firing, stale `window.appearance()`, or correct value but no visible change).

## Fix the localized break
- [ ] If the platform callback does not fire / reports a stale value on reverse: fix the Linux appearance watcher in `crates/gpui/src/platform/linux/**` (XDG `org.freedesktop.appearance` `color-scheme` portal handler + the window's cached `WindowAppearance`) so every OS change updates the cache and re-dispatches `appearance_changed`.
- [ ] Verify the fix dispatches to all live windows, not just the focused one.
- [ ] Remove the temporary logging once the break is fixed.

## Defensive consistency (latent dedup desync)
- [ ] Make the appearance-driven reload path and the `observe_global::<SettingsStore>` dedup caches (`prev_theme_name`/`prev_icon_theme_name`, theme_settings.rs ~70-152) share one source of truth so they cannot desync after a direct `reload_theme()`.

## Regression test (required — this is why prior fixes regressed)
- [ ] Extend `TestWindow`/`TestPlatform` to store the `on_appearance_changed` callback (currently a no-op at platform/test/window.rs:293) and add a `set_appearance(WindowAppearance)` test helper that invokes it.
- [ ] Add a test: mode=`System` with distinct light/dark themes; simulate light→dark→light and assert the active theme is correct after each transition (including the final revert to light).
- [ ] Add an assertion that a `Static` theme is NOT changed by simulated OS toggles.

## Verify & document
- [ ] Build: `cargo build --features external_websocket_sync -p zed`.
- [ ] Run the new test and the existing theme/gpui tests; confirm all pass.
- [ ] Manually verify light→dark→light→dark works repeatedly in a running build with no restart.
- [ ] Confirm icon theme (dynamic `System` mode) also follows both directions.
- [ ] Record any modified upstream files in `portingguide.md` per fork rebase convention.
- [ ] Open PR following CLAUDE.md hygiene (imperative title, `Release Notes:` section).
