# Design

## Root Cause

The `.markdown-body` styling in `DesignReviewContent.tsx` (lines 1292-1375) defines styles for headings, paragraphs, lists, blockquotes, code, and pre — but has **no `& a` rule**. Without it, links inherit the browser's default `#0000EE` dark blue, which is invisible on the dark background.

Same gap exists in `DesignDocPage.tsx` (lines 174-223).

## Fix

Add `& a` styling to both components, matching the pattern already used in `CodeIntelligenceTab.tsx`:

```tsx
// In DesignReviewContent.tsx, inside "& .markdown-body": { ... }
"& a": {
  color: "#00d5ff",         // Helix teal — matches CodeIntelligenceTab
  textDecoration: "none",
  "&:hover": {
    textDecoration: "underline",
  },
  "&:visited": {
    color: "#00d5ff",
  },
},
```

For `DesignDocPage.tsx`, use MUI theme-aware approach since it doesn't have a theme reference:

```tsx
// In the sx prop of the Box wrapping ReactMarkdown
'& a': {
  color: '#00d5ff',
  textDecoration: 'none',
  '&:hover': {
    textDecoration: 'underline',
  },
  '&:visited': {
    color: '#00d5ff',
  },
},
```

## Key Decisions

- **Use `#00d5ff` (teal)** not `#bbb` — teal is the established link color in Helix (see `CodeIntelligenceTab.tsx`, `tealRoot` in `themes.tsx`), has better contrast, and is visually distinct as a clickable element.
- **Pin visited color** to prevent browser default purple visited state.
- **No need for light/dark branching** — `#00d5ff` has good contrast on both the dark `background.paper` (~#121212) and light backgrounds. The `DesignReviewContent` component always uses `oneLight` syntax highlighting theme, suggesting it's primarily used in a light-on-dark-chrome context.

## Files to Change

| File | Line | Change |
|------|------|--------|
| `frontend/src/components/spec-tasks/DesignReviewContent.tsx` | ~1375 (before closing `}` of `.markdown-body`) | Add `& a` rule |
| `frontend/src/pages/DesignDocPage.tsx` | ~222 (before closing `}` of `sx`) | Add `& a` rule |
