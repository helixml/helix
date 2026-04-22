# Design: Pin Project from Project Detail Page

## Overview

Add a pin/unpin toggle button to the project detail page (`SpecTasksPage`) toolbar. All backend API endpoints and frontend React Query hooks already exist — this is purely a UI wiring task.

## Existing Infrastructure (No Changes Needed)

| Layer | Location | What Exists |
|-------|----------|-------------|
| **Backend API** | `api/pkg/server/project_handlers.go` | `pinProject()` (POST), `unpinProject()` (DELETE), `getPinnedProjects()` (GET) |
| **Frontend API client** | `frontend/src/api/api.ts` | `v1ProjectsPinCreate()`, `v1ProjectsPinDelete()`, `v1UsersMePinnedProjectsList()` |
| **React Query hooks** | `frontend/src/services/projectService.ts` | `usePinnedProjectIds()`, `usePinProject()`, `useUnpinProject()` |
| **Icons** | `@mui/icons-material` | `PushPin` (filled), `PushPinOutlined` |

## What To Change

### File: `frontend/src/pages/SpecTasksPage.tsx`

**Button placement:** In the toolbar's right-aligned `Stack` (line ~812), add the pin button between `ProjectMembersBar` and the action buttons `Box`. This keeps it near other project-level actions while not interfering with session-specific buttons.

**Implementation pattern:**

```tsx
// At top of component, add hooks:
const { data: pinnedProjectIds = [] } = usePinnedProjectIds(isLoggedIn);
const pinProjectMutation = usePinProject();
const unpinProjectMutation = useUnpinProject();
const isPinned = pinnedProjectIds.includes(project?.id || '');

// In toolbar JSX, add before action buttons:
<Tooltip title={isPinned ? "Unpin project" : "Pin project"}>
  <IconButton
    size="small"
    onClick={() => {
      if (isPinned) {
        unpinProjectMutation.mutate(project.id);
      } else {
        pinProjectMutation.mutate(project.id);
      }
    }}
    sx={{
      display: { xs: 'none', md: 'flex' },
      flexShrink: 0,
      color: isPinned ? '#a78bfa' : 'text.secondary',
    }}
  >
    {isPinned ? <PushPin /> : <PushPinOutlined />}
  </IconButton>
</Tooltip>
```

## Key Decisions

- **Same icon and color as projects list:** Uses `#a78bfa` (purple) for pinned state to stay consistent with `ProjectsListView.tsx` (line ~207).
- **Same React Query hooks:** The `usePinProject` / `useUnpinProject` hooks already handle cache invalidation, so the projects list will reflect changes made from the detail page without extra work.
- **Hidden on mobile:** Matches the responsive pattern used by other toolbar items (`display: { xs: 'none', md: 'flex' }`).
- **No loading state needed:** The mutations are fast and the hooks use optimistic `setQueryData` updates.

## Codebase Patterns Discovered

- The project detail page is `SpecTasksPage.tsx` (~1400 lines), rendered at route `/orgs/:org_id/projects/:id/specs`.
- Toolbar buttons follow a consistent pattern: `IconButton` with `size="small"`, `flexShrink: 0`, wrapped in `Tooltip`.
- The page uses both `@mui/icons-material` and `lucide-react` icons — the pin icons come from MUI, matching the projects list.
- The `isLoggedIn` variable is already available in `SpecTasksPage` via `useAccount()` hook.
