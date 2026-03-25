# Move view-mode toggle to left of topbar, immediately after breadcrumbs

## Summary
The Board / Split-Screen / Audit Trail toggle buttons now sit on the left side of the topbar, immediately after the project breadcrumbs, making them feel like primary page navigation rather than utility actions. The share/invite controls and other secondary actions remain on the right.

## Changes
- `AppBar.tsx`: Added optional `leftContent` prop; renders in a new `Cell` between the breadcrumb title and the right-side children
- `Page.tsx`: Added optional `topbarLeftContent` prop; threads through to `AppBar.leftContent`
- `SpecTasksPage.tsx`: Moved the view-mode toggle Stack (Board / Split-Screen / Audit Trail) from `topbarContent` into `topbarLeftContent`; invite/share controls remain in `topbarContent`
