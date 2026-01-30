# Design: Move Project to Organization

## Summary

Add a new API endpoint to move a project from a user's personal workspace into an organization. This enables RBAC and team sharing for previously personal projects.

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