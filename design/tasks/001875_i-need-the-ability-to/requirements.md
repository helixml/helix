# Requirements: Pin Project from Project Detail Page

## User Stories

**As a user viewing a project's detail page**, I want to pin/unpin the project directly from that page, so I don't have to navigate back to the projects list to pin it.

## Acceptance Criteria

- [ ] A pin/unpin toggle button is visible in the project detail page toolbar (header area)
- [ ] The button uses the same pin icon style as the projects list (`PushPinIcon` filled when pinned, `PushPinOutlinedIcon` when unpinned)
- [ ] Clicking the button pins an unpinned project and unpins a pinned project
- [ ] The button state reflects the current pin status immediately (optimistic update via React Query cache)
- [ ] The pin state stays in sync — pinning from the detail page is reflected when navigating back to the projects list
- [ ] The button includes a tooltip ("Pin project" / "Unpin project")
- [ ] The button is hidden on mobile viewports (consistent with other toolbar actions)

## Out of Scope

- No backend changes needed — `POST /api/v1/projects/{id}/pin` and `DELETE /api/v1/projects/{id}/pin` already exist
- No new hooks needed — `usePinnedProjectIds`, `usePinProject`, `useUnpinProject` already exist in `projectService.ts`
