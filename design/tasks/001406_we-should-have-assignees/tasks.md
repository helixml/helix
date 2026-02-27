# Implementation Tasks

## Backend

- [ ] Add `AssigneeID` field to `SpecTask` struct in `api/pkg/types/simple_spec_task.go`
- [ ] Add `AssigneeID` field to `SpecTaskUpdateRequest` struct (pointer type for nullable)
- [ ] Update `updateTask` handler in `api/pkg/server/spec_driven_task_handlers.go` to handle assignee updates
- [ ] Add validation: assignee must be org member (query org membership before saving)
- [ ] Run `./stack update_openapi` to regenerate API client

## Frontend

- [ ] Create `AssigneeSelector.tsx` component in `frontend/src/components/tasks/`
  - Popover with org members list
  - "Unassigned" option
  - Search filter for large teams
- [ ] Update `TaskCard.tsx` to display assignee avatar
  - Add avatar in card footer (near existing agent avatar)
  - Click handler to open AssigneeSelector
  - Tooltip with assignee name on hover
- [ ] Add React Query hook to fetch org members for current project's organization
- [ ] Wire up `updateSpecTask` mutation for assignee changes
- [ ] Update `SpecTaskWithExtras` interface to include `assignee_id`

## Testing

- [ ] Test assigning a member to a task
- [ ] Test unassigning (setting to null)
- [ ] Test reassigning to different member
- [ ] Verify assignment persists after page reload
- [ ] Verify non-org-member cannot be assigned (API validation)