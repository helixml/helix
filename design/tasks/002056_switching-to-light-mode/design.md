# Design: Fix Light/Dark Theme Toggle Not Returning to Dark Mode

## Root Cause

The bug lives in `crates/theme_settings/src/settings.rs:292-315`, in the `Static(_)` arm of `ThemeSelection::set_mode()`:

```rust
*selection = ThemeSelection::Dynamic {
    mode: ThemeAppearanceMode::System,         // BUG: ignores the `mode` arg
    light: ThemeName(DEFAULT_LIGHT_THEME.into()),
    dark:  ThemeName(DEFAULT_DARK_THEME.into()),
};
```

When the user's `theme` setting is `Static("SomeTheme")` and they invoke `theme::ToggleMode` (e.g. requesting `Dark`), the handler at `crates/workspace/src/workspace.rs:7973-7994` passes the requested mode to `set_mode()` — but the `Static(_)` branch discards it, hardcodes `System`, and overwrites both theme slots with built-in defaults.

Consequences:
1. The persisted `mode` becomes `System`, so the active theme is now governed by the OS appearance — not the user's explicit toggle.
2. The user's previously-chosen theme name is lost.
3. The next toggle starts from a different effective mode than the user expects, producing the symptom: "switching back to dark mode doesn't switch it back."

## Theme Settings Model (Reference)

`ThemeSettingsContent.theme: Option<ThemeSelection>` (`crates/settings_content/src/theme.rs:159`) has two shapes:
- `Static(ThemeName)` — one theme regardless of OS appearance.
- `Dynamic { mode: ThemeAppearanceMode, light: ThemeName, dark: ThemeName }`.

`ThemeAppearanceMode` is `Light | Dark | System`. Resolution at `crates/theme_settings/src/settings.rs:148-161`: `Light`/`Dark` pick the matching slot directly; `System` defers to the `SystemAppearance` global, refreshed by `cx.observe_window_appearance()` (`workspace.rs:1759-1766`).

## Fix

In the `Static(_)` arm of `set_mode()`:

1. Use the `mode` parameter passed to `set_mode()` instead of hardcoding `ThemeAppearanceMode::System`.
2. Preserve the existing static theme name by placing it into the slot matching its appearance (`Theme::appearance()` returns Light or Dark):
   - If the previous static theme is a light theme → put it in `light`, default the `dark` slot.
   - If it is a dark theme → put it in `dark`, default the `light` slot.
3. When `mode == System`, the same preservation logic still applies so the user does not silently lose their chosen theme.

Sketch:

```rust
ThemeSelection::Static(prev_name) => {
    let prev_theme = theme_registry.get(&prev_name.0).ok();
    let prev_is_dark = prev_theme.map(|t| t.appearance() == Appearance::Dark).unwrap_or(false);
    let (light, dark) = if prev_is_dark {
        (ThemeName(DEFAULT_LIGHT_THEME.into()), prev_name.clone())
    } else {
        (prev_name.clone(), ThemeName(DEFAULT_DARK_THEME.into()))
    };
    *selection = ThemeSelection::Dynamic { mode, light, dark };
}
```

## Symmetric Audit

- `set_theme()` at `settings.rs:233-264` performs an analogous Static→Dynamic conversion. Audit it for the same hardcoded-`System` defect and align behaviour.
- `IconThemeSelection::set_mode()` (icon themes mirror the same `Static | Dynamic` shape). Apply the same fix if present.

## Key Decisions

- **Preserve the user's previous theme rather than reset to defaults.** Reset-to-defaults was almost certainly unintentional. Preserving it matches user expectations and is what the existing tests imply when they exercise round-trip toggling.
- **No settings migration.** The on-disk `Dynamic` shape is unchanged; only the path that *produces* a new `Dynamic` from a `Static` changes. Existing settings files load identically.
- **No new actions, no new fields.** The fix is local to two functions.

## Risks / Open Questions

- If the previous static theme name does not resolve in `ThemeRegistry` (e.g. an uninstalled extension theme), `appearance()` lookup fails — fall back to defaulting both slots, same as today.
- The System-appearance observer at `workspace.rs:1759-1766` is correct as written; the fix relies on it only triggering when `mode == System`. Confirm no test asserts the observer overrides explicit modes.
- Determine whether `IconThemeSelection::set_mode` has the identical defect during implementation; if so, fix in the same PR to keep the two settings shapes behaviourally consistent.

## Reference Files

- `/home/retro/work/zed/crates/theme_settings/src/settings.rs:292-315` — primary fix site
- `/home/retro/work/zed/crates/theme_settings/src/settings.rs:233-264` — `set_theme()` audit site
- `/home/retro/work/zed/crates/theme_settings/src/settings.rs:148-161` — `ThemeSelection::name()` resolution (no change needed)
- `/home/retro/work/zed/crates/workspace/src/workspace.rs:7973-7994` — `toggle_theme_mode()` caller (no change needed)
- `/home/retro/work/zed/crates/workspace/src/workspace.rs:15696-15758` — existing test to extend
- `/home/retro/work/zed/crates/settings_content/src/theme.rs:159,267-280,344-354` — settings shape reference
- `/home/retro/work/zed/crates/theme/src/theme.rs:130-183` — `SystemAppearance` global
