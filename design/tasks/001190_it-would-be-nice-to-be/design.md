# Design: Pin Projects

## Architecture Overview

Pinning is a per-user preference stored in the database. A project can be pinned by multiple users independently — it is not a property of the project itself.

## Storage

**Extend `UserConfig`** (in `api/pkg/types/authz.go`) to add a `PinnedProjectIDs` field:

```go
type UserConfig struct {
  StripeSubscriptionActive bool
  StripeCustomerID         string
  StripeSubscriptionID     string
  PinnedProjectIDs         []string `json:"pinned_project_ids,omitempty"`
}
```

`UserConfig` is stored as JSON in the `user_meta.config` column (already JSONB). No migration is needed — the new field is additive and backward-compatible. This matches the existing pattern for user-specific settings.

**Decision:** Extending `UserConfig` was chosen over a new `user_project_pins` join table because:
- No schema migration required
- Consistent with existing user metadata storage
- Pin lists are small (few projects per user)
- The prompt history pin pattern (separate column) doesn't apply here because pins are per-user, not per-resource

## API

Two new endpoints on the existing projects router:

```
POST /api/v1/projects/{id}/pin    — Pin the project for the current user
DELETE /api/v1/projects/{id}/pin  — Unpin the project for the current user
```

Both endpoints:
1. Load the authenticated user's `UserMeta` (via `EnsureUserMeta`)
2. Modify `Config.PinnedProjectIDs` (append or remove the project ID)
3. Save via `UpdateUserMeta`
4. Return 200 OK

The existing `GET /api/v1/projects` list response already includes all projects. The frontend handles pinned ordering using the pinned IDs from a separate call or the user profile.

**Alternative considered:** Return `pinned: bool` on each project in the list response. Rejected as it requires a join/lookup per project on the backend; cleaner to let the frontend cross-reference.

## Frontend

### Data flow

1. `useListProjects` — existing hook, fetches all projects
2. New `useGetUserMeta` hook (or reuse if it already exists) — fetches `user_meta.config.pinned_project_ids`
3. `ProjectsListView` splits projects into `pinned` and `unpinned` arrays based on the IDs, renders a "Pinned" section header above pinned cards
4. New `usePinProject` / `useUnpinProject` mutations call the new endpoints with optimistic updates

### UI changes in `ProjectsListView`

- If any projects are pinned, render a "Pinned" section with a pin icon header, followed by the normal list
- Each project card gets a pin toggle button (a `PushPin` icon from MUI) in its action menu or card header
- Pinned cards show a filled pin icon; unpinned cards show an outline pin icon

### Key files to change

| File | Change |
|------|--------|
| `api/pkg/types/authz.go` | Add `PinnedProjectIDs` to `UserConfig` |
| `api/pkg/server/project_handlers.go` | Add `pinProject` / `unpinProject` handlers |
| `api/pkg/server/server.go` (or router file) | Register new routes |
| `frontend/src/services/projectService.ts` | Add `usePinProject`, `useUnpinProject` hooks |
| `frontend/src/services/userService.ts` (or similar) | Expose pinned IDs from user meta |
| `frontend/src/components/project/ProjectsListView.tsx` | Split pinned/unpinned, render sections |
| `frontend/src/components/project/ProjectCard.tsx` (if exists) | Add pin toggle button |

## Codebase Patterns

- **Existing pin reference:** `api/pkg/store/store_prompt_history.go` → `UpdatePromptPin`; follow the same handler style in `project_handlers.go`
- **UserMeta store methods:** `api/pkg/store/store_users.go` — use `EnsureUserMeta` + `UpdateUserMeta`
- **React Query hooks:** follow the pattern in `frontend/src/services/projectService.ts` (`useListProjects`, etc.)
- **JSONB serialization:** `gorm:"type:jsonb;serializer:json"` — `UserConfig` is already stored as JSON, no gorm tag change needed
- **Optimistic updates:** use React Query's `onMutate`/`onError`/`onSettled` pattern (check `promptHistoryService.ts` if it uses optimistic updates)
