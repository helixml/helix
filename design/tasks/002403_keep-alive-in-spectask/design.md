# Design: Make Keep Alive Toggle Green in Light and Dark Mode

## Scope

One file, one `sx` prop:
`frontend/src/components/tasks/SpecTaskDetailContent.tsx` lines ~2281-2306.

Verified by grep that this is the **only** Keep Alive UI control in the codebase
(`grep -rn -i "keep.alive|keepAlive" frontend/src` → only this component plus
generated `api.ts` types and unrelated WebSocket keepalive comments).
`helix-next` has no keep-alive UI at all.

## Approach

Replace the mode-agnostic `success.main` with a mode-aware green, using the
`useLightTheme()` hook already imported and in use in this component
(`lightTheme.isLight` is used for the panel resize handle a few hundred lines
above).

```tsx
// Keep Alive ON must read as green in BOTH modes. MUI's default
// success.main is green[800] (#2e7d32) in light mode — dark enough that it
// looks like just another dark toolbar icon next to text.secondary. Use a
// more saturated green for light mode; keep MUI's green[400] for dark.
const keepAliveOnColor = lightTheme.isLight ? "#1e8e3e" : "#66bb6a";
...
sx={{ color: task.keep_alive ? keepAliveOnColor : "text.secondary" }}
```

Define `keepAliveOnColor` once near the other derived render values in the
component body (not inside the JSX), so it isn't recomputed inline in the tree.

## Colour choice

Contrast computed against the toolbar background, which is
`backgroundColor: "background.paper"` (`#ffffff` light, `#121212` dark):

| Mode  | Value | Contrast vs paper | WCAG 1.4.11 (≥3:1) |
|-------|-------|-------------------|--------------------|
| light | `#1e8e3e` | 4.2:1 | pass |
| dark  | `#66bb6a` | ~8:1 | pass (unchanged) |

Rejected for light mode:
- `#2e7d32` (current, green 800) — 5.1:1 but perceptually muddy; too close in
  darkness to `text.secondary`, which is the actual defect.
- `#4caf50` (green 500) — 2.8:1 on white, **fails** 3:1 for an 18px icon.

## Alternatives considered

1. **Override `palette.success` globally in `contexts/theme.tsx`.** Rejected:
   blast radius across every success chip/alert/button for a one-icon ticket.
2. **Add `lightSuccess`/`darkSuccess` to `ITheme` in `themes.tsx` and expose
   `successColor` from `useLightTheme()`.** This is the "proper" reusable
   version and matches how `icon`/`iconHover`/`highlightColor`/`panelColor` are
   already handled. Rejected as the default only because it's a bigger change
   than this ticket needs — but it is the right move if a second component needs
   the same green. Flagged as Open Question 4; easy to switch to if the reviewer
   prefers it.
3. **Use `themeConfig.greenRoot` (`#3BF959`).** Rejected: that token is a
   dark-mode-only neon green with ~1.6:1 contrast on white. Several components
   (`DiffContent.tsx`, `DiffFileList.tsx`) use it unconditionally and likely
   have the same class of light-mode bug — out of scope here, worth a follow-up.

## Codebase notes (for future agents)

- **Theme structure.** `frontend/src/themes.tsx` holds the raw brand token table
  (`ITheme`), with explicit `light*` / `dark*` pairs. `contexts/theme.tsx` builds
  the MUI theme from those tokens — it only overrides `primary`, `secondary`,
  `background.default`, typography and a handful of component slots. **It does
  not override `success` / `error` / `warning` / `info`, so those fall through to
  MUI v5 defaults, which are different colours per mode.** That's the root cause
  of this bug class: `success.main` is green 800 in light and green 400 in dark.
- **The established pattern for mode-aware colour** is `useLightTheme()`
  (`frontend/src/hooks/useLightTheme.tsx`) returning `isLight` plus resolved
  `icon`/`textColor`/`panelColor`/etc. Inline `lightTheme.isLight ? a : b`
  ternaries are common in `SpecTaskDetailContent.tsx` and are accepted style.
- **Prior art for this exact bug class:** `0b8887200` "make desktop viewer
  chrome light-mode aware", `ba8fdc58a` "make Tests tab theme-aware to fix
  dark-on-dark in light mode", `c817cd5e2` "repair light-mode contrast in queue
  header and tooltip". Read those diffs before doing another light-mode fix.
- **Toolbar location.** The Keep Alive button lives in the *split-view right
  panel* header (`Panel defaultSize={70}` → view-toggle header `Box`), alongside
  Start / Stop / Restart / Upload. It renders only when `isDesktopRunning`.

## Verification plan

Per `helix/CLAUDE.md`, prefer end-to-end verification in the inner Helix at
`http://localhost:8080` (register `test@helix.ml` / `helixtest`, complete
onboarding, create a spec task, open its detail page and wait for the desktop to
run). Vite HMR means no rebuild is needed for the edit itself.

1. With the desktop running, screenshot the toolbar with Keep Alive OFF, click
   it, screenshot with Keep Alive ON — in **dark** mode.
2. Toggle to light mode (theme toggle / OS `prefers-color-scheme`) and repeat.
3. Save all four to
   `helix-specs/design/tasks/002403_keep-alive-in-spectask/screenshots/`.
4. `cd frontend && yarn build` before committing.

If the desktop cannot be brought to a running state, say so explicitly rather
than claiming visual verification — a colour change is only verified by looking
at it in both modes.
