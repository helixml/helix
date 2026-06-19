# Design: System / Light / Dark Three-Way Theme Toggle in Settings

## Architecture Overview

The theme system lives in `frontend/src/contexts/theme.tsx` (`ThemeProviderWrapper`).
It holds a `mode: PaletteMode` state, a live `matchMedia` OS listener, and exposes
`{ mode, toggleMode }` via `ThemeContext`. All components read the resolved `mode`
via MUI's `useTheme()` or the `useLightTheme` / `useThemeConfig` hooks.

The current binary `toggleMode()` function and context shape need to be extended to
support a three-value preference while still only ever passing `'light'` or `'dark'`
into MUI's `createTheme`.

## Key Decisions

**1. New preference type**

Add `type ThemePreference = 'system' | 'light' | 'dark'` alongside the existing
`PaletteMode`. The `ThemeContext` will expose:

```ts
{
  mode: PaletteMode            // resolved: 'light' | 'dark' — what MUI uses
  themePreference: ThemePreference   // stored user choice
  setThemePreference: (p: ThemePreference) => void
}
```

`toggleMode` is removed (or kept as a backward-compat alias cycling through the three
options) — the UI calls `setThemePreference` directly.

**2. Persistence — localStorage key `helixColorScheme`**

The existing code already removed the old `themeMode` key at startup to kill stale
binary state. The new key `helixColorScheme` stores `'system' | 'light' | 'dark'`.
Absent key → treated as `'system'` (identical to current behaviour, so no regression).

`getInitialPreference()` reads this key.
`getInitialMode()` resolves preference → `PaletteMode` using current OS state.

**3. OS listener conditionality**

The `matchMedia` `'change'` handler only calls `setMode` when
`themePreference === 'system'`. Explicit preferences are unaffected by OS changes.

**4. API sync**

`v1UsersMeColorSchemeUpdate` continues to receive the *resolved* mode (`'light'` or
`'dark'`), unchanged. If a future API needs the raw preference that is a separate concern.

## Files to Change

| File | Change |
|------|--------|
| `frontend/src/contexts/theme.tsx` | Add `ThemePreference` type, `themePreference` state, `setThemePreference`, update OS listener guard, update `ThemeContext` shape |
| `frontend/src/components/account/GeneralSettings.tsx` | Add three-way `ToggleButtonGroup` UI reading/setting `ThemeContext` |

No other files need changing — all downstream consumers read `mode` via MUI's
`useTheme()` which is unaffected.

## UI Component

A MUI `ToggleButtonGroup` (exclusive) with three `ToggleButton` entries:
- `'system'` — monitor/computer icon + label "System"
- `'light'` — sun icon + label "Light"  
- `'dark'` — moon icon + label "Dark"

Placed inside a new `Grid` panel row in `GeneralSettings.tsx` using the same
`panelBg` / `borderRadius: 2` styling as the existing panels.

## Patterns Found in Codebase

- `ThemeContext` is the single source of truth; components should not read
  `localStorage` directly for theme — they consume context.
- `useLightTheme` resolves `isLight` from `theme.palette.mode` (MUI) — no changes
  needed there since we still supply only `'light'` or `'dark'` to MUI.
- Existing panels in `GeneralSettings.tsx` follow the pattern:
  `<Grid container spacing={2} sx={{ mt: 2, backgroundColor: panelBg, p: 2, borderRadius: 2 }}>`.
