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

Top-level structure (relevant ranges):

```tsx
// line 1142
<Box className="prompt-input-container" data-prompt-input="true" sx={{ position: 'relative' }}>
  {/* Queued messages display — sibling above the input, the "queue ... above it" the user reported */}
  // lines 1147–1234
  <Collapse in={showQueue && queuedMessages.length > 0}>
    <Box sx={{ mb: 1.5, borderRadius: 1.5, border: '1px solid', ..., overflow: 'hidden' }}>
      <Box>{/* header: icon + "Message queue (saved locally)" + count chip */}</Box>
      <Box sx={{ maxHeight: 200, overflowY: 'auto' }}>
        {/* SortableQueueItem rows: drag handle + status icon + truncated text + edit/delete buttons */}
      </Box>
    </Box>
  </Collapse>

  ... attachment chips ...

  {/* Bordered input container */}
  // lines 1356–1622
  <Box sx={{ display: 'flex', flexDirection: 'column', ..., p: 1 }}>
    <Box component="textarea" sx={{ width: '100%', ... }} />
    {/* Buttons row */}
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1 }}>...</Box>
  </Box>

  {/* Keyboard hint (below the input, not what this task is about) */}
</Box>
```

`SortableQueueItem` (lines 228–254 and 348–389) already declares `minWidth: 0` on its content `Box` plus `overflow: hidden, textOverflow: ellipsis, whiteSpace: nowrap` on the preview `Typography`. There is an existing in-code comment at lines 359–363 referencing a prior bug where the queue item's actions box was pushed past the queue container's `overflow: hidden` clip on mobile — that fix was applied *inside* each row but did not address the queue container itself overflowing its parent.

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

### Issue 1 — overflow on mobile chat (input box and queue above it)

Two compounding causes:

1. **Missing `min-width: 0` on the flex child wrapping `RobustPromptInput`.** In CSS flexbox, a flex item's default `min-width` is `auto`, which equals its content's min-content width. The `RobustPromptInput` outer `<Box>` (line 1142) does not declare `width: 100%` or `minWidth: 0`, so it measures at its content's natural min-content width. The two children that contribute to that natural min-width are:
   - The **queue panel** (lines 1147–1234). The queue header is a `display: flex` row whose icon + text + chip don't shrink past their natural sizes; the queue-item rows have icons + buttons + ellipsised text. The header text "Message queue (saved locally)" or "Offline - saved locally, will send when connected" is fairly wide.
   - The **bordered input container** (lines 1356–1622). The action-buttons row at line 1424 does not declare `flexWrap: 'wrap'`, so a long row of icon buttons (history, attach, camera, interrupt-mode, send, etc.) contributes a fixed min-content width too.
   The wrapping `<Box sx={{ flex: 1 }}>` (line 2741) inherits the flexbox default `min-width: auto`, so it refuses to shrink below the inner content's min-width. The parent chat panel does set `overflow: hidden`, but that only *clips* the overflow — it doesn't make the content fit. That clip is exactly what the user is seeing: "slightly overflow … and also are not scrollable."
2. **Buttons row inside `RobustPromptInput` (line 1424) does not set `flexWrap: 'wrap'`.** Even with the parent flex fix, very narrow viewports with many optional buttons can still push the buttons row's min-content width past the available space. Letting it wrap on narrow widths removes that contribution from the min-width calculation.

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

### D2 — Make `RobustPromptInput`'s outer wrapper claim full width and contain its own min-width

Change line 1142 of `RobustPromptInput.tsx` from:

```tsx
<Box className="prompt-input-container" data-prompt-input="true" sx={{ position: 'relative' }}>
```

to:

```tsx
<Box className="prompt-input-container" data-prompt-input="true" sx={{ position: 'relative', width: '100%', minWidth: 0 }}>
```

This guarantees the component fills its parent and never measures wider than its parent, regardless of the embedding context (chat panel, split view, anywhere else it's used). With this change the queue panel and the bordered input container inherit a constrained width and stop being able to push past the right edge.

### D3 — Allow the action-buttons row to wrap on narrow widths

Change line 1424 of `RobustPromptInput.tsx` from:

```tsx
<Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1 }}>
```

to:

```tsx
<Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 1, flexWrap: 'wrap' }}>
```

This protects against the buttons row contributing a large fixed min-content width on very narrow viewports when many optional buttons are present (history + attach + camera + interrupt-mode + send + others). Visually unchanged on widths where the icons already fit on one row, which is the common case.

### D4 — Add top padding and increase horizontal padding to the mobile search bar

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

- **Use `overflow-x: auto` on the chat input wrapper to make it scrollable** — rejected. The user explicitly said "fix it so that it actually fits within the width of the screen on mobile". Horizontal scrolling for an input field and its queue is a poor mobile UX; the right answer is to make the content fit.
- **Hide the queue display entirely on mobile or change its header text to something shorter** — rejected. The queue is essential UX (drag-to-reorder, edit, delete pending messages, restart on crash) and hiding it on mobile would regress functionality. The fix is to constrain its width, not to hide content.
- **Set `width: 100vw` somewhere in the chain** — rejected. `100vw` ignores parent paddings/scrollbars and is a frequent source of overflow bugs. The flexbox `min-width: 0` idiom is the correct fix.
- **Use `useMediaQuery` to fork mobile vs. desktop sx for the search bar** — unnecessary; the existing `display: { xs: "flex", md: "none" }` block is already mobile-only, so changing `pt`/`px` directly inside it is enough.

## Notes for future agents

- **Flexbox `min-width: 0` rule:** any time you have `display: flex` and a child whose content can be wider than the parent (long text, fixed-width chips, icon rows), the child must declare `minWidth: 0` (for row direction) or `minHeight: 0` (for column direction). This is the #1 cause of "my MUI component overflows on mobile" in this codebase. Look at `SortableQueueItem` (RobustPromptInput.tsx lines 293, 348, 364, 373) — it already follows this pattern internally, with an in-code comment explaining a past mobile bug fixed by adding `minWidth: 0`. The same fix needs to be applied one level up, on the wrapper around `RobustPromptInput` itself.
- **Mobile vs. desktop layouts in `SpecTaskDetailContent.tsx`:** there are two parallel render paths — `isBigScreen` (lines ~1781+) and the mobile path (lines ~2664+). Layout fixes must be considered for both, but they are physically separate JSX trees, so changes to one do not automatically affect the other.
- **Mobile detection in this file:** `isBigScreen` comes from `useIsBigScreen({ breakpoint: "md" })` (line 160), and the initial `currentView` uses `window.matchMedia("(max-width: 899.95px)")` (line 329). These are consistent — both use the MUI `md` breakpoint of 900px.
- **`RobustPromptInput` is reused** in multiple places (chat panel, split view, possibly other session views). Any width-related change to its outer wrapper must be parent-agnostic — that's why D2 uses `width: 100%, minWidth: 0` rather than viewport units.
- **Terminology check:** the user's bug report used the word "cue" for what is really the **queue** (queued-messages display) above the input. This component is rendered at `RobustPromptInput.tsx` lines 1147–1234. There is also a keyboard-shortcut *hint* row below the input (lines 1624–1667) — that is a different element, not in scope for this task.

## Implementation Notes

- **`yarn build` blocked by read-only `dist/`.** `frontend/dist` is bind-mounted into the prod frontend container as `:ro` (see CLAUDE.md "Production Frontend Mode"). When dev is in the production-frontend mode, `vite build` fails with `EACCES: mkdir frontend/dist/external-libs`. For type-check verification only, run `yarn tsc` instead (it runs `tsc -b tsconfig.json`, no disk writes other than the build-info file). Vite-HMR-served dev mode picks up source changes automatically — no build needed for inner-Helix testing.
- **All four code edits applied cleanly**, line numbers from the design held after the `git merge origin/main` step.
- **Inner Helix at `localhost:8080` not reachable** from this agent's environment during the implementation pass (`curl` returns 000). Visual testing in the running inner Helix was not possible. Type-check is green; the changes are isolated `sx` prop additions on three components — no behavioural surface touched. See screenshots/ folder if produced.
