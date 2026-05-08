# Design: Light Mode for the Helix Frontend

## Current State

The theming infrastructure is **partially built** but unused:

| Component | Status | File |
|-----------|--------|------|
| `ThemeContext` with `mode` state + `toggleMode()` | Exists, hardcoded to `'dark'` | `src/contexts/theme.tsx` |
| Light/dark color pairs in theme config | Defined (light* and dark* properties) | `src/themes.tsx` |
| `useLightTheme()` hook | Works, returns mode-aware colors | `src/hooks/useLightTheme.tsx` |
| Toggle UI | **Missing** | — |
| localStorage persistence | **Missing** | — |
| OS preference detection | **Missing** | — |
| MUI component overrides | Hardcoded to dark colors | `src/contexts/theme.tsx` |

### Scope of Hardcoded Dark Colors

- **50 files** already use `useLightTheme()` — these should mostly work once the mode changes
- **31 files** access `themeConfig.dark*` directly (147 occurrences) — need migration to `useLightTheme()`
- **27 files** have hardcoded dark hex values (#121214, #1e1e24, etc.) — need extraction
- **73 files** have inline color properties — many will need light-mode variants
- Worst offenders: `MonacoEditorImpl.tsx` (24), `PasswordResetComplete.tsx` (15), `ImportAgent.tsx` (18), `SessionToolbar.tsx` (10)

## Architecture

### Decision: Extend Existing Infrastructure

**Chose to extend** the existing `ThemeContext` / `useLightTheme` / `themes.tsx` pattern rather than introducing a new system (e.g., CSS variables or a separate theme library).

**Why:** The pattern already exists across 50 files. Adding CSS variables would create two competing systems. MUI's `createTheme()` already supports mode-aware palettes natively.

### Theme Mode Flow

```
User clicks toggle
  → ThemeContext.toggleMode()
  → mode state flips ('dark' ↔ 'light')
  → localStorage.setItem('themeMode', newMode)
  → MUI createTheme() rebuilds with new mode
  → All components using useTheme()/useLightTheme() re-render
```

### Initialization Priority

1. Check `localStorage.getItem('themeMode')` — user's explicit choice
2. Check `window.matchMedia('(prefers-color-scheme: light)')` — OS preference
3. Default to `'dark'` (current behavior, no breaking change)

### Toggle UI Placement

Add a light/dark toggle icon button in the top app bar (next to existing controls). Uses `LightMode` / `DarkMode` MUI icons. Simple, discoverable, standard placement.

### MUI Component Override Strategy

The MUI component overrides in `theme.tsx` (MuiMenu, MuiDialog, MuiPaper, MuiPopover) currently hardcode dark colors like `rgba(26, 26, 26, 0.97)`, `#181A20`, `white`. These need to become mode-conditional:

```typescript
MuiMenu: {
  styleOverrides: {
    root: {
      '& .MuiMenu-list': {
        backgroundColor: mode === 'light'
          ? 'rgba(255, 255, 255, 0.97)'
          : 'rgba(26, 26, 26, 0.97)',
        // ...
      }
    }
  }
}
```

### Light Mode Color Palette

Reuse the existing `light*` values from `themes.tsx`:

| Token | Light Value | Purpose |
|-------|-------------|---------|
| `lightBackgroundColor` | `#ffffff` | Page background |
| `lightText` | `#333` | Primary text |
| `lightTextFaded` | `#aeaeae` | Secondary text |
| `lightIcon` | `#5d5d7b` | Icon color |
| `lightPanel` | `#f4f4f4` | Panel/card backgrounds |
| `lightBorder` | `1px solid #aeaeae` | Borders |
| `lightHighlight` | `#00d5ff` | Highlights/accents |

Additional light-mode tokens needed in `themes.tsx`:
- `lightScrollbar`, `lightScrollbarThumb`, `lightScrollbarHover` — scrollbar styling
- Light-mode dialog/menu surface colors (or derive from MUI defaults)

### Migration Strategy for Components

**Phase 1 — Core:** Fix theme context, add toggle, make MUI overrides mode-aware. This makes the 50 files using `useLightTheme()` work immediately.

**Phase 2 — Direct theme access:** Migrate 31 files that use `themeConfig.dark*` directly to use `useLightTheme()` instead.

**Phase 3 — Hardcoded colors:** Extract hardcoded hex values from 27 files into theme-aware patterns.

**Phase 4 — Inline styles:** Address remaining inline color references in components (best-effort; focus on visible/high-impact surfaces first).

### Codebase Patterns Found

- **Routing uses react-router5** — `useRouter()` with `router.navigate('name', { params })`
- **State management is MobX** — theme state lives in React context, not MobX (appropriate since it's UI-only state)
- **Styling is MUI `sx` prop + Emotion** — no Tailwind, no CSS modules of significance
- **`useLightTheme()` returns:** `{ isLight, isDark, border, backgroundColor, icon, textColor, textColorFaded, scrollbar }`
- **`useThemeConfig()` returns** the full `ITheme` object with all 60+ tokens
- **Font: IBM Plex Sans** — imported from Google Fonts in `index.html`

### Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Some components may look broken in light mode on first pass | Prioritize high-traffic pages (session, dashboard, create). Use visual QA checklist. |
| Monaco editor has its own theme system | Monaco already supports `vs-light` theme — switch based on mode |
| Charts may become unreadable | Chart gradient colors are already separate tokens; verify contrast |
| `index.html` has hardcoded dark `<meta>` theme-color | Make it dynamic or set a neutral value |
