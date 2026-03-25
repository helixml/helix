# Design: Board View Split-Screen and Audit Trail Button Grouping

## Current Architecture

**AppBar** (`frontend/src/components/system/AppBar.tsx`)
```
Row
├── Cell flexShrink:1   ← breadcrumb title lives here
└── Cell grow end       ← children (topbarContent) rendered here, right-aligned
```

**Page** (`frontend/src/components/system/Page.tsx`)
- Renders breadcrumbs as `useTopbarTitle` → passed to `AppBar` as `title`
- Accepts `topbarContent` prop → passed to `AppBar` as `children` (right-aligned)

**SpecTasksPage** (`frontend/src/pages/SpecTasksPage.tsx`, lines 697–835)
- Puts the view-mode toggle `Stack` inside `topbarContent`, so it lands on the right

## Proposed Change

Add a `topbarLeftContent` prop to both `Page` and `AppBar`. This slot renders immediately after the breadcrumb title and before the spacer/grow cell, so it appears on the left side of the topbar next to the breadcrumbs.

```
Row
├── Cell flexShrink:1   ← breadcrumb title (unchanged)
├── Cell flexShrink:0   ← NEW: topbarLeftContent (view toggle goes here)
└── Cell grow end       ← topbarContent, right-aligned (share/invite etc.)
```

**`AppBar.tsx`** — add optional `leftContent?: React.ReactNode` prop; render it in a new `Cell` between the title and children.

**`Page.tsx`** — add optional `topbarLeftContent?: ReactNode` prop (line 23 area); thread it through to `AppBar` as `leftContent`.

**`SpecTasksPage.tsx`** — move the view-mode toggle `Stack` (lines 698–835) out of `topbarContent` into `topbarLeftContent`.

## Implementation Notes

- The old `topbarContent` in `SpecTasksPage` was a single outer `<Stack direction="row" spacing={2}>` containing both the invite button AND the toggle Stack AND other buttons. The refactor splits this into two props.
- `topbarLeftContent` receives a bare `<Stack>` (the toggle group) — no outer wrapper needed.
- `topbarContent` now holds a new `<Stack direction="row" spacing={2}>` wrapping the invite button and the remaining action buttons — preserving the original layout.
- Build verified clean with `yarn build` (no TypeScript errors).

## Key Decisions

- **Minimal surface change**: Only two system components (`AppBar`, `Page`) need new optional props. No structural refactor.
- **Backwards-compatible**: Both props are optional; all other pages are unaffected.
- **No styling changes**: The toggle `Stack` keeps its existing active-state styling. The only change is its position in the row.
- **Separator**: No visual separator is required — the natural left/right positioning conveys the grouping. If one is needed later it can be added as a `Divider` or `Box` with `borderLeft`.

## Patterns in this Codebase

- MUI `Stack` / `Box` / `Cell` for layout; `Cell` with `grow end` = right-align.
- `topbarContent` prop pattern already used by many pages; this follows the same pattern for the left side.
- Responsive hiding uses `sx={{ display: { xs: 'none', md: 'flex' } }}` — the existing split-screen button already does this; leave it alone.
