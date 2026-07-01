# Design: Fix Tests Tab Dark-on-Dark Colors in Light Mode (Agent Settings)

## Root Cause

`frontend/src/components/app/TestsEditor.tsx` hardcodes dark colors that ignore
the active theme mode:

| Location (approx. line) | Hardcoded value | Used for |
|---|---|---|
| Test card `sx` (~224) | `backgroundColor: '#2a2d3e'` | per-test card |
| Step card `sx` (~249) | `backgroundColor: '#1e1e2f'` | per-step card |
| CLI instructions box (~316) | `backgroundColor: '#2a2d3e'` | outer info panel |
| CLI command box (~326) | `backgroundColor: '#1e1e2f'` | command snippet |
| Copy-command IconButton (~343) | `color: 'white'` | copy icon |
| GitHub/GitLab accordions (~373, ~418) | `backgroundColor: '#1e1e2f'` | accordion |
| GitHub/GitLab code boxes (~384, ~429) | `backgroundColor: '#0d1117'` | code block |
| Copy-config IconButtons (~397, ~441) | `color: 'white'` | copy icons |

MUI text colors follow the theme, so in light mode text is dark while these
backgrounds stay dark → unreadable.

## Codebase Patterns Found

- The project already has a theme hook: `frontend/src/hooks/useLightTheme.tsx`.
  It exposes `isLight`, `panelColor`, `backgroundColor`, `textColor`,
  `textColorFaded`, `border`, `icon`, `iconHover`, and a `scrollbar` sx.
  `panelColor`/`backgroundColor` resolve to the correct value for the current
  mode (light vs dark) from `useThemeConfig()`.
- Idiom (see `components/app/CalculatorSkill.tsx`): `const lightTheme =
  useLightTheme()` then `sx={{ background: lightTheme.panelColor, borderTop:
  lightTheme.border }}`.
- Many sibling skill components under `components/app/` already use
  `useLightTheme`, so this is the established fix.

## Approach

Replace the hardcoded hex colors and `color: 'white'` in `TestsEditor.tsx` with
theme-aware values via the existing `useLightTheme` hook. Keep the dark-mode
appearance identical while giving light mode legible colors.

Mapping:
- Import and call `useLightTheme()` at the top of the component.
- Card / panel / accordion backgrounds (`#2a2d3e`, `#1e1e2f`) →
  `lightTheme.panelColor` (or a nested darker variant using
  `lightTheme.backgroundColor` for the inner code/command boxes) so the two-tier
  visual nesting is preserved in both modes.
- Code snippet boxes (`#0d1117`) → in dark mode keep a dark code background; use
  `lightTheme.isLight ? <light-code-bg> : '#0d1117'` (a light neutral such as a
  `themeConfig` panel value) so code blocks stay readable in light mode.
- Copy icon buttons `color: 'white'` → `lightTheme.icon` (theme-aware); hover
  background stays subtle/theme-neutral.

Prefer resolving all colors through `useLightTheme` / `useThemeConfig` values
rather than introducing new hex literals. If a distinct "code block" tone is
needed that the hook doesn't expose, use an `isLight` conditional with existing
theme-config values instead of adding one-off hex codes.

## Key Decisions

- **Reuse `useLightTheme` rather than inline `theme.palette.mode` checks** — it
  is the established project pattern and centralizes the color values.
- **Preserve visual nesting** (test card > step card, panel > code box) by using
  two tones (`panelColor` for outer, `backgroundColor` for inner) so the layout
  reads the same in both modes.
- **No functional/layout changes** — this is purely a color/theming fix, minimizing
  regression risk.

## Testing

- Build the frontend: `cd frontend && yarn build`.
- End-to-end in the inner Helix (`http://localhost:8080`): register/login, open
  an agent's settings → Tests tab, toggle light mode (Appearance settings), and
  confirm all text is readable. Then toggle dark mode and confirm no visual
  regression. Capture before/after screenshots in both modes.

## Files to Change

- `frontend/src/components/app/TestsEditor.tsx` (only file).
