# Design: Fix Mobile Overflow on Spec Task Chat & Padding on Mobile Search Bar

## Files & current state

### 1. Mobile chat view container
**File:** `frontend/src/components/tasks/SpecTaskDetailContent.tsx`

The mobile chat view (rendered when `currentView === "chat"` on viewports ≤ 899.95px) is structured as:

```tsx
// lines ~2664–2763
<Box sx={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0, overflow: "hidden" }}>
  ... thread selector ...
  <EmbeddedSessionView ref={sessionViewRef} sessionId={activeSessionId} />
  <Box sx={{ p: 1.5, flexShrink: 0, display: "flex", alignItems: "flex-start", gap: 1 }}>
    <Box sx={{ flex: 1 }}>                                          {/* <-- problem */}
      <RobustPromptInput ... />
    </Box>
  </Box>
</Box>
```

### 2. RobustPromptInput component
**File:** `frontend/src/components/common/RobustPromptInput.tsx`

- Outer wrapper (line 1142): `<Box sx={{ position: 'relative' }}>` — no explicit `width: 100%`.
- Input border container (line 1356): `<Box sx={{ display: 'flex', flexDirection: 'column', ..., p: 1 }}>`.
- Textarea (line 1382): `width: '100%'`.
- Buttons row (line 1424): `display: 'flex', alignItems: 'center', gap: 0.5, mt: 1` — does not declare `flexWrap`, so on narrow widths the action icons can push the row past the container's right edge.
- Keyboard-hint cue row (lines 1624–1667): `display: 'flex', justifyContent: 'center', gap: 2, flexWrap: 'wrap'`. Each hint is a `<Box display: flex, gap: 0.5>` containing a 12px icon + caption text. `flexWrap: 'wrap'` is set, which is correct, but the row still contributes to the parent flex's natural min-content width.

### 3. Mobile search bar in kanban board
**File:** `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx`

```tsx
// lines 1652–1693
<Box sx={{
  display: { xs: "flex", md: "none" },
  flexShrink: 0,
  px: 1,        // <-- only 8px horizontal padding
  pb: 1,        // <-- 8px bottom padding
  // NO pt/py — bumped flush against the top of its container
  gap: 1,
  alignItems: "center",
}}>
  <TextField fullWidth ... />
</Box>
```

The desktop header above (lines 1479–1650) uses `display: { xs: "none", md: "flex" }`, so on mobile the search bar is the first thing rendered in the kanban container.

## Root cause analysis

### Issue 1 — overflow on mobile chat

Two compounding causes:

1. **Missing `min-width: 0` on the flex child wrapping `RobustPromptInput`.** In CSS flexbox, a flex item's default `min-width` is `auto`, which equals its content's min-content width. If the content (the prompt input + button row + cue row) has any element that does not shrink (e.g. a non-wrapping caption, a long icon row), the flex item refuses to shrink below it and the whole layout overflows the parent. Even though the parent has `overflow: hidden`, the child can still be wider than the parent — the overflow is just clipped, which exactly matches the user's report ("overflow … and also are not scrollable").
2. **Buttons row inside `RobustPromptInput` (line 1424) does not set `flexWrap: 'wrap'`.** On narrow widths, the row of action icons (history, attach, camera, send, etc.) can push past the container width. The keyboard-hint row (line 1625) does set `flexWrap: 'wrap'`, but its `gap: 2` (16px) is still relatively wide for very small viewports.

### Issue 2 — search bar padding

Plain omission. The mobile search container only sets `px: 1, pb: 1`. With the desktop header hidden, the input visually touches the top edge of the kanban container, and the 8px horizontal padding doesn't match the more generous spacing used elsewhere on the page.

## Decisions

### D1 — Add `minWidth: 0` to the chat-view prompt-input wrapper

Change line 2741 of `SpecTaskDetailContent.tsx` from:

```tsx
<Box sx={{ flex: 1 }}>
```

to:

```tsx
<Box sx={{ flex: 1, minWidth: 0 }}>
```

This is the canonical CSS-flexbox fix for "child overflows its flex parent". It is safe because:
- The parent flex direction is `row`, the wrapper is the only sibling, so `minWidth: 0` lets it occupy `100%` of the parent and shrink with it.
- Desktop layout (`isBigScreen`, lines 1938–1964) uses a different wrapper structure (`flex: 1` directly on the `RobustPromptInput`-containing Box inside a resizable panel) and is not affected by this change.

### D2 — Make `RobustPromptInput`'s outer wrapper claim full width and contain its own overflow

Change line 1142 of `RobustPromptInput.tsx` from:

```tsx
<Box className="prompt-input-container" data-prompt-input="true" sx={{ position: 'relative' }}>
```

to:

```tsx
<Box className="prompt-input-container" data-prompt-input="true" sx={{ position: 'relative', width: '100%', minWidth: 0 }}>
```

This guarantees the component fills its parent and never measures wider than its parent, regardless of the embedding context (chat panel, split view, anywhere else it's used).

### D3 — Allow the action-buttons row to wrap on narrow widths

Change line 1424 of `RobustPromptInput.tsx` from:

```tsx
<Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1 }}>
```

to:

```tsx
<Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1, flexWrap: 'wrap' }}>
```

This protects against horizontal overflow when many optional buttons are present (history + attach + camera + interrupt-mode + send + others) on very narrow viewports. Visually unchanged on widths where the icons already fit on one row (which is the common case).

### D4 — Tighten the keyboard-hint cue row gap on narrow viewports

The keyboard-hint row at lines 1625–1634 currently uses `gap: 2` (16px). Change to a responsive value that uses a smaller gap on `xs`:

```tsx
sx={{
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  gap: { xs: 1, sm: 2 },
  rowGap: { xs: 0.5, sm: 1 },
  mt: 0.75,
  px: 0.5,
  flexWrap: 'wrap',
}}
```

Smaller `gap` on mobile reduces row width per chip and lets more hint chips share a row before wrapping; the explicit `rowGap` keeps wrapped rows visually tight.

### D5 — Add top padding and increase horizontal padding to the mobile search bar

Change lines 1654–1661 of `SpecTaskKanbanBoard.tsx` from:

```tsx
sx={{
  display: { xs: "flex", md: "none" },
  flexShrink: 0,
  px: 1,
  pb: 1,
  gap: 1,
  alignItems: "center",
}}
```

to:

```tsx
sx={{
  display: { xs: "flex", md: "none" },
  flexShrink: 0,
  pt: 2,
  px: 2,
  pb: 1,
  gap: 1,
  alignItems: "center",
}}
```

`pt: 2` (16px) gives breathing room from the top edge. `px: 2` (16px) matches typical mobile content padding and prevents the input from feeling cropped against the screen edges.

## Alternatives considered

- **Use `overflow-x: auto` on the chat input wrapper to make it scrollable** — rejected. The user explicitly said "fix it so that it actually fits within the width of the screen on mobile". Horizontal scrolling for an input field is a poor mobile UX; the right answer is to make the content fit.
- **Hide the keyboard-hint cue row entirely on mobile** — considered. Mobile users typing on a touch keyboard rarely use Ctrl+Enter / Shift+Enter shortcuts. However, hiding content removes information; the simpler win is letting it wrap correctly. If after the layout fix the cue still feels visually noisy, a follow-up can hide it conditionally — out of scope for this task.
- **Set `width: 100vw` somewhere in the chain** — rejected. `100vw` ignores parent paddings/scrollbars and is a frequent source of overflow bugs. The flexbox `min-width: 0` idiom is the correct fix.
- **Use `useMediaQuery` to fork mobile vs. desktop sx for the search bar** — unnecessary; the existing `display: { xs: "flex", md: "none" }` block is already mobile-only, so changing `pt`/`px` directly inside it is enough.

## Notes for future agents

- **Flexbox `min-width: 0` rule:** any time you have `display: flex` and a child whose content can be wider than the parent (long text, fixed-width chips, icon rows), the child must declare `minWidth: 0` (for row direction) or `minHeight: 0` (for column direction). This is the #1 cause of "my MUI component overflows on mobile" in this codebase. Search for `flex: 1` near `RobustPromptInput`, `EmbeddedSessionView`, and other large widgets to find candidates.
- **Mobile vs. desktop layouts in `SpecTaskDetailContent.tsx`:** there are two parallel render paths — `isBigScreen` (lines ~1781+) and the mobile path (lines ~2664+). Layout fixes must be considered for both, but they are physically separate JSX trees, so changes to one do not automatically affect the other.
- **Mobile detection in this file:** `isBigScreen` comes from `useIsBigScreen({ breakpoint: "md" })` (line 160), and the initial `currentView` uses `window.matchMedia("(max-width: 899.95px)")` (line 329). These are consistent — both use the MUI `md` breakpoint of 900px.
- **`RobustPromptInput` is reused** in multiple places (chat panel, split view, possibly other session views). Any width-related change to its outer wrapper must be parent-agnostic — that's why D2 uses `width: 100%, minWidth: 0` rather than viewport units.
