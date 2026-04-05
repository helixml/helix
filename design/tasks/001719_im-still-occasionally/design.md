# Design: Fix Link Colors in Spec Review Dark Mode

## Problem

Links rendered by `ReactMarkdown` in the spec review section use browser-default colors (dark blue `#0000FF` unvisited, dark purple `#800080` visited). These are nearly invisible on the dark grey backgrounds (`#121214` base, `background.paper` for the markdown container).

## Root Cause

Both markdown rendering components define `sx` styles for headings, paragraphs, lists, code, blockquotes, etc. — but completely omit `& a` anchor tag styles. No global anchor override exists in the MUI theme either.

## Affected Files

1. **`frontend/src/components/spec-tasks/DesignReviewContent.tsx`** (~line 1287) — the `"& .markdown-body"` sx block
2. **`frontend/src/pages/DesignDocPage.tsx`** (~line 174) — the `Box` sx block

## Fix

Add `"& a"` styles to both markdown containers' `sx` props. Use the theme's teal accent (`#00D5FF` / `primary.main` maps to `#8989a5`, so use a direct teal value or `secondary` depending on theme — the teal `#00D5FF` is already established as an accent color in the theme via `tealRoot`).

Example addition to the existing sx objects:

```tsx
"& a": {
  color: "#00D5FF",
  textDecoration: "none",
  "&:hover": {
    textDecoration: "underline",
  },
  "&:visited": {
    color: "#00D5FF",
  },
},
```

Alternatively, use MUI theme tokens: `color: "primary.main"` — but `primary.main` is `#8989a5` (muted purple), which may still be low-contrast. The teal `#00D5FF` is a better choice for readability.

## Decision: Use teal `#00D5FF` directly

- Matches the existing accent color used elsewhere in the UI (hover highlights use `rgba(0,229,255,0.13)`)
- High contrast against dark backgrounds (WCAG AAA on `#121214`)
- `primary.main` (`#8989a5`) is too muted for link text on dark backgrounds

## Light Mode Consideration

The fix uses a hardcoded dark-mode color. If light mode is ever used for these pages, consider wrapping in a theme-aware conditional. For now, the app defaults to dark mode and the spec review is only used in dark mode.
