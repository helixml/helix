# Design: Move Project to Organization

## Summary

Add a new API endpoint to move a project from a user's personal workspace into an organization, enabling RBAC and team sharing for previously personal projects. Include UI in the Danger Zone section of project settings.

## Architecture

### Current State

The `Project` struct already has an `OrganizationID` field:

```go
// helix/api/pkg/types/project.go
type Project struct {
    ID             string `json:"id" gorm:"primaryKey"`
    UserID         string `json:"user_id" gorm:"index"`         // Owner
    OrganizationID string `json:"organization_id" gorm:"index"` // Empty for personal
    // ...
}
```

The authorization system (`authorizeUserToProject` in `authz.go`) already handles both cases:
- `OrganizationID == ""`: Personal project, only owner can access
- `OrganizationID != ""`: Org project, uses RBAC via `authorizeUserToResource`

### Solution

Add a dedicated `POST /api/v1/projects/{id}/move` endpoint that:
1. Validates user owns the project
2. Validates user is a member of target org
3. Updates project's `OrganizationID`
4. Updates associated git repositories' `OrganizationID`
5. Updates `ProjectRepository` junction table entries

## API Design

### Request

```
POST /api/v1/projects/{id}/move
Content-Type: application/json

{
  "organization_id": "org_01abc..."
}
```

### Response

```json
{
  "id": "prj_01xyz...",
  "name": "My Project",
  "organization_id": "org_01abc...",
  "user_id": "usr_01def..."
}
```

### Errors

| Status | Condition |
|--------|-----------|
| 400 | Missing or empty `organization_id` |
| 403 | User is not project owner |
| 403 | User is not member of target org |
| 404 | Project not found |

## UI Design

### Location

Add "Move to Organization" section in the **Danger Zone** of `ProjectSettings.tsx`, but **only for personal projects** (where `project.organization_id` is empty/null).

### Pattern

Follow the existing Danger Zone pattern in `ProjectSettings.tsx` (lines 1085-1120):

```tsx
{/* Move to Organization - only show for personal projects */}
{!project?.organization_id && (
  <Box sx={{
    p: 2,
    backgroundColor: 'rgba(211, 47, 47, 0.05)',
    borderRadius: 1,
    border: '1px solid',
    borderColor: 'error.light',
    mb: 2
  }}>
    <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
      Move to Organization
    </Typography>
    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
      Transfer this project to an organization to enable team sharing and RBAC roles.
      This is a one-way operation and cannot be undone.
    </Typography>
    {/* Organization dropdown + Move button */}
  </Box>
)}
```

### Components

1. **Organization Dropdown**: Use MUI `Select` with user's organizations from `account.organizationTools.organizations`
2. **Move Button**: Outlined error button, disabled until org selected
3. **Confirmation Dialog**: Warn about one-way operation, show target org name

### Data Flow

1. Get user's organizations from `useAccount()` hook (`account.organizationTools.organizations`)
2. On move button click, show confirmation dialog
3. Call generated API client method (after running `./stack update_openapi`)
4. On success, invalidate project query to refresh data
5. Show success snackbar

## Data Changes

### Tables Updated

1. **projects** - Set `organization_id`
2. **git_repositories** - Update `organization_id` for repos linked to this project
3. **project_repositories** - Update `organization_id` in junction table

### No Migration Needed

All affected tables already have `organization_id` columns. This is a runtime data update, not a schema change.

## Implementation Notes

### Pattern: Follow existing project handlers

Location: `helix/api/pkg/server/project_handlers.go`

The handler should follow the same patterns as `updateProject`:
- Use `getRequestUser(r)` and `getID(r)` helpers
- Use `authorizeUserToProject` for ownership check
- Use `authorizeOrgMember` for org membership check
- Return `*types.Project, *system.HTTPError` like other handlers

### Associated Resources

Resources that reference `ProjectID` but DON'T need `OrganizationID` updates:
- `SpecTask` - Accesses project via `authorizeUserToProjectByID`
- `Session` - Already has its own `OrganizationID` field set at creation
- `ProjectAuditLog` - Historical records, don't update
- `PromptHistoryEntry` - Historical records, don't update
- `LLMCall` / `UsageMetric` - Historical records, don't update

### Transaction Safety

Wrap all updates in a database transaction to ensure atomicity. If updating git repositories fails, roll back the project update.

## Decisions

| Decision | Rationale |
|----------|-----------|
| New endpoint vs extend update | Dedicated endpoint makes the operation explicit and allows different authorization rules |
| Keep UserID unchanged | Original owner retains ownership, org membership provides team access |
| Update git repo org IDs | Ensures consistent RBAC for repo access within the org |
| Don't update historical records | Audit logs and metrics should reflect state at time of creation |
| Danger Zone placement | Matches existing pattern, signals irreversible action |
| One-way move only | Simplifies implementation; moving back to personal is rare use case |