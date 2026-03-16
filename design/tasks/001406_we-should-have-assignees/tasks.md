# Implementation Tasks

## Backend

- [x] Add `AssigneeID` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [x] Add `AssigneeID` field to `SpecTaskUpdateRequest` struct (pointer type for nullable)
- [x] Update `updateTask` handler in `api/pkg/server/spec_driven_task_handlers.go` to handle assignee updates
- [x] Add validation: assignee must be org member (query org membership before saving)
- [x] Run `./stack update_openapi` to regenerate API client

## Frontend

- [x] Create `AssigneeSelector.tsx` component in `frontend/src/components/tasks/`
  - Popover with org members list
  - "Unassigned" option
  - Search filter for large teams
- [x] Update `TaskCard.tsx` to display assignee avatar
  - Add avatar in card footer (near existing agent avatar)
  - Click handler to open AssigneeSelector
  - Tooltip with assignee name on hover
- [x] Add React Query hook to fetch org members for current project's organization
  - Note: Using existing account.organizationTools.organization.memberships
- [x] Wire up `updateSpecTask` mutation for assignee changes
- [x] Update `SpecTaskWithExtras` interface to include `assignee_id`

## Testing

- [x] Test assigning a member to a task
- [x] Test unassigning (setting to null)
- [x] Test reassigning to different member
- [x] Verify assignment persists after page reload
- [x] Verify non-org-member cannot be assigned (API validation)