# Design: Shareable Design Doc Page Scroll Fix

## Overview

Fix the scroll issue on the shareable design doc page (`/design-doc/:specTaskId/:reviewId`) by wrapping the content in a scrollable container.

## Current Architecture

```
Layout.tsx
└── Box (overflow: 'hidden')  ← Blocks scrolling
    └── DesignDocPage.tsx
        └── Container (py: 4)  ← No scroll handling
            └── Paper sections...
```

Pages that need scrolling use the `Page` component, which provides `overflowY: 'auto'` on its content area. `DesignDocPage.tsx` bypasses this and renders directly.

## Solution

Add `overflow: 'auto'` and `height: '100%'` to the root `Container` in `DesignDocPage.tsx`.

This is the minimal fix approach, consistent with how `PasswordReset.tsx` handles scroll (uses a root Box with `minHeight: '100vh'`).

## Alternative Considered

**Wrap in `Page` component**: Would require adding breadcrumb navigation and would change the standalone nature of the shareable page. Rejected for simplicity since this is meant to be a minimal public-facing view.

## Code Change

In `helix/frontend/src/pages/DesignDocPage.tsx`, modify the root Container:

```tsx
// Before
<Container maxWidth="lg" sx={{ py: 4 }}>

// After
<Container maxWidth="lg" sx={{ py: 4, height: '100%', overflow: 'auto' }}>
```

## Files Modified

- `helix/frontend/src/pages/DesignDocPage.tsx` - Add scroll styles to Container