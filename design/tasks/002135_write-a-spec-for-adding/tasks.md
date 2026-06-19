# Implementation Tasks: System / Light / Dark Three-Way Theme Toggle in Settings

- [ ] In `frontend/src/contexts/theme.tsx`, add `type ThemePreference = 'system' | 'light' | 'dark'`
- [ ] Add `getInitialPreference()` reading `localStorage.getItem('helixColorScheme')`, defaulting to `'system'`
- [ ] Update `getInitialMode()` to accept the preference and resolve it to `PaletteMode` using `window.matchMedia`
- [ ] Replace `mode` state initialisation to derive from `getInitialPreference()` via `getInitialMode()`
- [ ] Add `themePreference` state (initialised from `getInitialPreference()`)
- [ ] Add `setThemePreference(p: ThemePreference)` that: saves to `localStorage`, updates `themePreference` state, resolves and updates `mode`, and calls `v1UsersMeColorSchemeUpdate` for the resolved mode
- [ ] Guard the `matchMedia` `'change'` handler so it only updates `mode` when `themePreference === 'system'`
- [ ] Update `ThemeContext` type and default value to include `themePreference` and `setThemePreference`; remove or alias `toggleMode`
- [ ] In `frontend/src/components/account/GeneralSettings.tsx`, consume `ThemeContext` to read `themePreference` and `setThemePreference`
- [ ] Add a labelled `ToggleButtonGroup` panel row (System / Light / Dark with icons) using the existing `panelBg` panel styling
