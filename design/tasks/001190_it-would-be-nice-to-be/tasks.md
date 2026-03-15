# Implementation Tasks

## Backend

- [x] Add `PinnedProjectIDs []string` to `UserConfig` in `api/pkg/types/authz.go`
- [~] Add `pinProject` handler in `api/pkg/server/project_handlers.go` — `POST /api/v1/projects/{id}/pin` (append project ID to user's pinned list)
- [~] Add `unpinProject` handler in `api/pkg/server/project_handlers.go` — `DELETE /api/v1/projects/{id}/pin` (remove project ID from user's pinned list)
- [~] Register the two new routes in the server router

## Frontend

- [ ] Add `usePinProject` and `useUnpinProject` mutation hooks in `frontend/src/services/projectService.ts`
- [ ] Expose `pinnedProjectIDs` from user meta in the frontend (add hook or selector to fetch/read from user profile)
- [ ] Update `ProjectsListView.tsx` to split projects into pinned/unpinned arrays and render a "Pinned" section header above pinned cards
- [ ] Add a pin toggle button (MUI `PushPin` icon) to each project card — filled when pinned, outlined when not
- [ ] Wire pin toggle to the mutation hooks with optimistic UI update
