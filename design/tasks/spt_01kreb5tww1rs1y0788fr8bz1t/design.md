# Design: Fix dark-on-dark text in message queue header in light mode

## Root cause

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

## Fix

Add a single `color` rule to the header `Box`'s `sx` that resolves to the
matching contrastText for whichever `.dark` background is active:

```tsx
color: editingId ? 'info.contrastText' : isOnline ? 'primary.contrastText' : 'warning.contrastText',
```

This works because:
- MUI's palette guarantees a readable `<token>.contrastText` for every palette
  color (it's computed against the `.main` shade; `.dark` is darker still, so
  the contrastText remains readable).
- `<Typography>` without an explicit color inherits from its parent via CSS
  `color` cascading.
- Lucide icons use `currentColor` for their stroke, so they pick up the same
  color automatically.
- The MUI `<Chip>` has its own background and text color, so it is unaffected.

## Alternatives considered

1. **Set `color` only on the `<Typography>` and pass `color` prop to each icon.**
   More verbose; misses the icons if a future contributor swaps the icon set.
   The parent-`Box` approach is the minimal, idiomatic MUI pattern.

2. **Use `primary.contrastText` unconditionally.** The three palette
   families could theoretically have different contrast values; using the
   matching contrastText for each state is safer and self-documenting.

3. **Switch the background to `.main` shades.** Would change the visual design
   (lighter header strip). Out of scope — the bug is the foreground, not the
   background.

## Files to change

- `frontend/src/components/common/RobustPromptInput.tsx` — one `sx` field added
  to the queue-header `Box` (around line 1161).

## Verification

Hot-reload via Vite (frontend container, port 8081 — no rebuild needed). Then,
in the inner Helix browser at `http://localhost:8080`:

1. Register `test@helix.ml` / `helixtest`, complete onboarding, open a session.
2. Switch the app to **light mode** (theme toggle in the user menu).
3. Open the prompt input and queue at least one message (type, Ctrl+Enter while
   another send is in flight, or queue while offline). The header should be
   legible.
4. Repeat for the **editing** state (click the pencil on a queued item) and the
   **offline** state (DevTools → Network → "Offline").
5. Toggle back to **dark mode** and confirm the header still looks correct
   (the contrastText should resolve to a light color in dark mode too, since
   `.dark` is dark in both modes).

Screenshot before/after both light and dark in
`screenshots/` in this task folder when implementing.
