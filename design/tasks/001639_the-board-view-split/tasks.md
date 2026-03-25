# Implementation Tasks

- [x] Add optional `leftContent?: React.ReactNode` prop to `AppBar` (`frontend/src/components/system/AppBar.tsx`); render it in a new `Cell` (flexShrink:0) between the title cell and the existing grow/end cell
- [x] Add optional `topbarLeftContent?: ReactNode` prop to `Page` (`frontend/src/components/system/Page.tsx`); pass it to `AppBar` as `leftContent`
- [x] In `SpecTasksPage.tsx`, move the view-mode toggle `Stack` (Board / Split-Screen / Audit Trail buttons, lines ~698–835) from `topbarContent` into `topbarLeftContent`
- [x] Verify the three buttons appear immediately right of the breadcrumbs and that share/invite controls remain on the right
- [x] Verify responsive behaviour: Split-Screen button still hidden at xs, others visible at all breakpoints
- [x] Run `cd frontend && yarn build` to confirm no TypeScript errors
