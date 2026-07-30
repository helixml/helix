# Requirements: Make Keep Alive Toggle Green in Light and Dark Mode

## Background

The spec-task detail page has a **Keep Alive** toggle in the right-hand content
panel's toolbar (shown only while the desktop is running). It's a lock icon:
`LockIcon` when Keep Alive is ON, `LockOpenIcon` when OFF.

Current implementation — `frontend/src/components/tasks/SpecTaskDetailContent.tsx:2281-2306`:

```tsx
<IconButton
  size="small"
  onClick={handleToggleKeepAlive}
  disabled={updateSpecTask.isPending}
  sx={{ color: task.keep_alive ? "success.main" : "text.secondary" }}
>
  {task.keep_alive ? <LockIcon .../> : <LockOpenIcon .../>}
</IconButton>
```

`success.main` is *not* overridden in `contexts/theme.tsx`, so it resolves to
MUI v5 defaults, which differ by mode:

| Mode  | `success.main` | Toolbar bg (`background.paper`) | Contrast |
|-------|----------------|----------------------------------|----------|
| dark  | `#66bb6a` (green 400) | `#121212` | ~8:1 — reads as bright green |
| light | `#2e7d32` (green 800) | `#ffffff` | ~5.1:1 — reads as a dark, muddy green |

In light mode the ON colour (`#2e7d32`) sits very close in perceived darkness to
the OFF colour (`text.secondary` = `rgba(0,0,0,0.6)`), so the toggle does not
read as "green / on" — it just looks like another dark toolbar icon. In dark mode
the same pair is unmistakable (bright green vs light grey).

## User Stories

**US1 — Keep Alive reads as green in light mode**
As a user viewing a spec task in light mode, I want the Keep Alive lock icon to
be clearly green when Keep Alive is ON, so I can tell at a glance that the
container won't auto-sleep.

Acceptance criteria:
- [ ] With `keep_alive = true` in light mode, the lock icon is a clearly
      saturated green — visually distinct from the neighbouring grey/dark toolbar
      icons and from the OFF state.
- [ ] The green has ≥3:1 contrast against `background.paper` (`#ffffff`),
      meeting WCAG 2.1 SC 1.4.11 for non-text graphical objects.

**US2 — Dark mode stays green (no regression)**
As a user in dark mode, I want the existing bright-green ON state preserved.

Acceptance criteria:
- [ ] With `keep_alive = true` in dark mode, the lock icon remains bright green
      (equivalent to today's `#66bb6a`), ≥3:1 against `#121212`.

**US3 — OFF state stays neutral in both modes**
Acceptance criteria:
- [ ] With `keep_alive = false`, the open-lock icon stays neutral
      (`text.secondary`) in both modes — the fix must not make OFF look ON.
- [ ] Tooltip text ("Keep Alive ON/OFF …") is unchanged.
- [ ] The toggle still switches colour immediately after clicking (optimistic /
      post-mutation refetch behaviour unchanged).

## Non-Goals

- Changing global `palette.success` — that would recolour every success chip,
  alert and button in the app. Out of scope for this ticket.
- Changing the icon choice, tooltip wording, placement, or the keep-alive
  backend behaviour.
- Fixing other light-mode contrast issues in the same toolbar (e.g. components
  using the dark-mode-only `themeConfig.greenRoot` neon `#3BF959`). Note them if
  spotted, but don't fix here.

## Open Questions

1. **Which mode is actually wrong?** The request says "should be green in both
   light and dark mode", implying one mode is currently not green. Static
   analysis says both modes technically render a green (`#2e7d32` light /
   `#66bb6a` dark), so the most likely complaint is that light mode's dark
   forest green does not *read* as green. The spec assumes **light mode is the
   broken one**. The first implementation task is to reproduce in both modes and
   confirm before changing values — if dark mode is the problem instead, the same
   mode-aware fix applies with the values swapped.
2. **Exact light-mode green?** Design proposes `#1e8e3e` (4.2:1 on white).
   Alternative: MUI green 700 `#388e3c` (4.1:1). Either is fine; happy to take a
   specific brand green if there's a preferred one.
3. **Should the ON state get more than colour** (e.g. a faint green tinted
   button background) so it's distinguishable without relying on hue alone?
   Assumed **no** — the request asks for green, and colour-only is consistent
   with the other toolbar toggles.
4. **Should this land as a reusable theme token** (`lightSuccess`/`darkSuccess`
   in `themes.tsx`, exposed via `useLightTheme()`) rather than an inline
   ternary? Assumed **inline**, matching the existing style in this same file —
   see design.md §Alternatives.
