# Implementation Tasks

- [~] In `frontend/src/pages/SpecTasksPage.tsx`, import `usePinnedProjectIds`, `usePinProject`, `useUnpinProject` from `projectService.ts` and `PushPin`, `PushPinOutlined` from `@mui/icons-material`
- [ ] In `SpecTasksPage` component body, add the three hooks and derive `isPinned` from `pinnedProjectIds.includes(project.id)`
- [ ] In the toolbar `Stack` (around line 835), add a pin/unpin `IconButton` with `Tooltip`, matching the style and color scheme from `ProjectsListView.tsx`
- [ ] Verify the pin button appears in the project detail page toolbar, toggles correctly, and syncs with the projects list view
