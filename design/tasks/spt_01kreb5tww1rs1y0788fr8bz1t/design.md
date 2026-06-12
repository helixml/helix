# Design: Fix light-mode contrast bugs in queue header and usage sparkline tooltip

## Bug 1 — Message queue header

### Root cause

`frontend/src/components/common/RobustPromptInput.tsx` lines 1161–1192 render
the queue header as:

```tsx
<Box
  sx={{
    ...
    bgcolor: editingId ? 'info.dark' : isOnline ? 'primary.dark' : 'warning.dark',
    ...
  }}
>
  {editingId ? <CirclePause size={16} /> : isOnline ? <Cloud size={16} /> : <CloudOff size={16} />}
  <Typography variant="caption" sx={{ flex: 1, fontWeight: 600 }}>
    {editingId ? 'Editing - paused from here' : isOnline ? 'Message queue (saved locally)' : 'Offline - saved locally, will send when connected'}
  </Typography>
  <Chip label={queuedMessages.length} size="small" sx={{ height: 18, fontSize: '0.7rem' }} />
</Box>
```

`primary.dark` / `info.dark` / `warning.dark` are intentionally dark in **both**
palette modes (that's the point of the `.dark` shade), so the background is
always dark. But the `<Typography>` and the lucide icons (which render SVGs
using `currentColor`) inherit MUI's default body text color, which in light mode
is near-black — hence dark-on-dark.

The matching pattern elsewhere in the file works correctly because it pairs the
dark background with an explicit contrast color — e.g. the history hint at lines
1247–1265 sets both `bgcolor: 'primary.main'` and `color: 'primary.contrastText'`.

### Fix

Add a single `color` rule to the header `Box`'s `sx` that resolves to the
matching contrastText for whichever `.dark` background is active:

```tsx
color: editingId ? 'info.contrastText' : isOnline ? 'primary.contrastText' : 'warning.contrastText',
```

This works because:
- MUI's palette guarantees a readable `<token>.contrastText` for every palette
  color (computed against the `.main` shade; `.dark` is darker still, so the
  contrastText remains readable).
- `<Typography>` without an explicit color inherits from its parent via CSS
  `color` cascading.
- Lucide icons use `currentColor` for their stroke, so they pick up the same
  color automatically.
- The MUI `<Chip>` has its own background and text color, so it is unaffected.

## Bug 2 — Usage sparkline hover tooltip

### Root cause

`frontend/src/components/usage/UsageSparkline.tsx` lines 123–160 render the
hover tooltip via a `<Popper>` containing a `<Paper>` with a hard-coded dark
background:

```tsx
<Paper sx={{
  p: 1,
  backgroundColor: 'rgba(30, 30, 30, 0.95)',
  border: '1px solid rgba(255,255,255,0.1)',
  borderRadius: 1,
}}>
  ...
  <Typography variant="caption" sx={{ color: 'text.secondary', ... }}>...</Typography>
  <Typography variant="caption" sx={{ color: 'text.primary', ... }}>...</Typography>
```

In light mode, MUI's `text.primary` / `text.secondary` are near-black / dark
gray, so they paint dark text on the still-dark `rgba(30, 30, 30, 0.95)`
background — exactly the "black on dark gray" the user reported.

The same component also has an SVG hover guideline at lines 101–110:

```tsx
<line ... stroke="rgba(255,255,255,0.5)" ... />
```

That hard-coded semi-transparent white is invisible against the card's
near-white stat-strip background in light mode.

### Fix

Use `useLightTheme()` (already widely used in the project — see
`ProjectsListView.tsx` for the established pattern) to switch the tooltip
chrome and the SVG guideline between dark and light variants:

```tsx
import useLightTheme from '../../hooks/useLightTheme'
...
const lightTheme = useLightTheme()
...

// Hover guideline (light-mode visible, dark-mode unchanged)
<line
  ...
  stroke={lightTheme.isLight ? 'rgba(0,0,0,0.4)' : 'rgba(255,255,255,0.5)'}
  ...
/>

// Tooltip paper
<Paper sx={{
  p: 1,
  backgroundColor: lightTheme.isLight ? 'rgba(255,255,255,0.97)' : 'rgba(30, 30, 30, 0.95)',
  border: lightTheme.isLight
    ? '1px solid rgba(0,0,0,0.10)'
    : '1px solid rgba(255,255,255,0.10)',
  borderRadius: 1,
  boxShadow: lightTheme.isLight ? '0 4px 16px rgba(0,0,0,0.12)' : undefined,
}}>
```

The light-mode surface, border, and shadow values mirror the conventions in
`contexts/theme.tsx` for `menuSurfaceBg` / `menuBorder` / `menuShadow`, so the
tooltip blends with other floating panels in light mode. The typography colors
inside (`text.primary` / `text.secondary`) become correct automatically because
MUI's palette already provides light-mode-appropriate dark text values — the
problem was the background, not the text tokens.

## Alternatives considered

1. **Set `color` only on `<Typography>` and pass `color` prop to each icon
   (Bug 1).** More verbose; misses the icons if a future contributor swaps the
   icon set. The parent-`Box` approach is the minimal, idiomatic MUI pattern.

2. **Use `primary.contrastText` unconditionally (Bug 1).** The three palette
   families could theoretically have different contrast values; using the
   matching contrastText for each state is safer and self-documenting.

3. **Force the tooltip to stay dark in both modes (Bug 2)** by also forcing
   light text inside. Doable, but inconsistent with the rest of the app: every
   other floating surface (menus, dialogs, popovers) switches with the palette
   in `contexts/theme.tsx`. The tooltip should follow suit.

4. **Switch the queue-header background to `.main` shades (Bug 1).** Would
   change the visual design (lighter header strip). Out of scope — the bug is
   the foreground, not the background.

## Files to change

- `frontend/src/components/common/RobustPromptInput.tsx` — one `sx` field added
  to the queue-header `Box` (around line 1161).
- `frontend/src/components/usage/UsageSparkline.tsx` — import `useLightTheme`,
  call it in the component body, and use it to gate `<Paper>` chrome (around
  lines 132–137) and the SVG hover guideline stroke (around line 107).

## Verification

Hot-reload via Vite (frontend container, port 8081 — no rebuild needed). Then,
in the inner Helix browser at `http://localhost:8080`:

### Queue header
1. Register `test@helix.ml` / `helixtest`, complete onboarding, open a session.
2. Switch the app to **light mode** (theme toggle in the user menu).
3. Queue at least one message and confirm the header label/icon are legible.
4. Repeat for the **editing** state (click pencil on a queued item) and the
   **offline** state (DevTools → Network → "Offline").
5. Toggle back to **dark mode** and confirm no regression.

### Sparkline tooltip
1. Still in light mode, navigate to the Projects list (cards view).
2. Hover over the sparkline area inside a card with usage data. Confirm the
   tooltip's date and numbers are legible and the vertical dashed line on the
   sparkline is visible.
3. Toggle to **dark mode** and confirm the tooltip still looks like before (dark
   surface with light text, faint white guideline).

Screenshot before/after both bugs in both modes into
`screenshots/` in this task folder when implementing.
