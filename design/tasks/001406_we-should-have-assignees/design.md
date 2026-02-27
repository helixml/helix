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