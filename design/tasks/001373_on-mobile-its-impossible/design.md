# Design: Mobile-Friendly Spec Review Page Tabs

## Overview

Hide the git commit info and timestamp on narrow screens to ensure the document tabs remain visible and usable on mobile devices.

## Component

`helix/frontend/src/components/spec-tasks/DesignReviewContent.tsx` (lines ~790-820)

## Current Structure

```
Header Bar (flexbox, space-between)
├── Left: Tabs (Requirements | Technical Design | Implementation Plan)
└── Right: Git info box
    ├── Git branch/commit chip (e.g., "main @ abc1234")
    ├── Timestamp (e.g., "1/15/2026, 3:45 PM")
    ├── Share button (icon)
    └── Comment log button (icon)
```

## Solution

Use MUI's responsive `display` prop to hide elements on narrow screens:

```tsx
// Git branch/commit chip - hide on mobile
<Chip
  sx={{
    display: { xs: 'none', sm: 'flex' },
    // ... existing styles
  }}
/>

// Timestamp - hide on mobile
<Typography
  sx={{
    display: { xs: 'none', sm: 'block' },
    // ... existing styles
  }}
>
```

## Breakpoint

- `xs`: 0-599px → Hide git chip and timestamp
- `sm`: 600px+ → Show all elements

## What Stays Visible on Mobile

- All three document tabs (primary navigation)
- Share button (small icon, useful for mobile sharing)
- Comment log button (small icon, needed for review workflow)

## Pattern Reference

This follows existing patterns in the codebase:
- `StartupScriptEditor.tsx` uses `display: { xs: 'none', sm: 'inline' }` to hide elements on mobile
- `SessionToolbar.tsx` uses similar responsive display patterns