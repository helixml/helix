# Design: Task Assignees on Kanban Board

## Overview
Add an `assignee_id` field to SpecTask and display assignee avatars on TaskCard components.

## Architecture

### Backend Changes

**1. Database Schema (`types/simple_spec_task.go`)**

Add to `SpecTask` struct:
```go
AssigneeID string `json:"assignee_id,omitempty" gorm:"size:255;index"`
```

Add to `SpecTaskUpdateRequest`:
```go
AssigneeID *string `json:"assignee_id,omitempty"` // Pointer to allow clearing
```

**2. API Changes**
- `PUT /api/v1/spec-tasks/{taskId}` already accepts `SpecTaskUpdateRequest` — just add the new field
- No new endpoints needed; organization members list already exists at `GET /api/v1/organizations/{id}/members`

### Frontend Changes

**1. TaskCard Component (`components/tasks/TaskCard.tsx`)**

Add assignee display in card footer:
- Show avatar (using `TypesUser` from org members)
- Click to open `AssigneeSelector` popover

**2. New Component: `AssigneeSelector.tsx`**

Simple popover with:
- List of org members (fetched via `v1OrganizationsMembersDetail`)
- "Unassigned" option at top
- Search/filter for large teams

**3. Update API Types**

The generated `api.ts` will include the new field after running `./stack update_openapi`.

## Data Flow

```
TaskCard click → AssigneeSelector popover → select member → 
  updateSpecTask mutation → optimistic update → refetch tasks
```

## Key Decisions

1. **Single assignee** — Simpler UX, matches common Kanban patterns (Trello, Linear)
2. **Organization members only** — Uses existing membership model, no new auth needed
3. **Nullable field** — Tasks can be unassigned (null/empty string)
4. **No notifications** — Out of scope for initial implementation

## Migration

- New `assignee_id` column added via GORM AutoMigrate (nullable VARCHAR)
- Existing tasks default to unassigned
- No data migration needed

## Security

- Assignee must be an organization member (validate on backend)
- Only users with project write access can change assignees
- Uses existing `authorizeUserToProjectByID` check in update handler

## Implementation Notes

### Files Modified

**Backend:**
- `api/pkg/types/simple_spec_task.go` - Added `AssigneeID` field to SpecTask and SpecTaskUpdateRequest
- `api/pkg/server/spec_driven_task_handlers.go` - Added assignee update handling with org membership validation

**Frontend:**
- `frontend/src/components/tasks/AssigneeSelector.tsx` - New component for assignee selection popover
- `frontend/src/components/tasks/TaskCard.tsx` - Added assignee avatar display and selector integration

### Patterns Used

- **Organization members from account context**: Reused `account.organizationTools.organization.memberships` instead of creating new API calls
- **Existing mutation pattern**: Used `useUpdateSpecTask` mutation which already handles task updates
- **Pointer type for nullable**: Used `*string` in Go to allow clearing assignee with empty string

### Gotchas

- The swagger-typescript-api generator needs `--extract-response-body --extract-request-body` flags (NOT `--no-client`) to preserve the Api class export
- TaskCard uses `useMemo` to find assigned member from org members list - avoids unnecessary re-renders