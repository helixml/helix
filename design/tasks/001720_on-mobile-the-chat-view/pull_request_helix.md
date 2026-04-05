# Default to chat view on mobile in spec task detail

## Summary
On mobile devices, the spec task detail page now defaults to the "chat" tab instead of "desktop". The desktop stream view is less useful on small screens, while chat is the primary interaction mode.

## Changes
- Updated `getInitialView()` in `SpecTaskDetailContent.tsx` to check screen width via `window.matchMedia` and return `"chat"` on screens below the `md` breakpoint (900px)
- Reuses the same 900px breakpoint already used for split-view switching (`isBigScreen`)
- Explicit `?view=` URL params are still respected regardless of screen size
