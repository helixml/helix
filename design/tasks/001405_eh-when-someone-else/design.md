# Design: Cross-User SpecTask Authorization Fix

## Overview

Fix authorization failures when org members (not the session owner) interact with SpecTask sessions.

## Current Behavior

```
User A creates spectask → Session created with Owner=UserA
User B (org member with project RBAC) → 403 on clipboard, 401 on resume
```

## Root Cause (Verified via Browser Testing)

The session has `organization_id` and `project_id` correctly set. The RBAC path in `authorizeUserToSession()` is being followed. However, **the default "write" role does not include the "Project" resource type**.

The "write" role config:
```json
{"rules": [{"effect": "allow", "actions": ["Get", "List", "UseAction", "Create", "Update", "Delete"], "resource": ["Application", "Knowledge"]}]}
```

But session authorization checks:
```go
return apiServer.authorizeUserToResource(ctx, user, session.OrganizationID, session.ProjectID, types.ResourceProject, action)
```

Since `"Project"` is not in `["Application", "Knowledge"]`, the RBAC check fails.

## Solution

Update the default role configurations to include `"Project"` in the allowed resources list.

### Code Changes

**Location:** Find where default roles are created (likely during org creation or in migrations)

Update the "write" role to:
```json
{
  "rules": [{
    "effect": "allow",
    "actions": ["Get", "List", "UseAction", "Create", "Update", "Delete"],
    "resource": ["Application", "Knowledge", "Project"]
  }]
}
```

Similarly update "read" and "admin" roles as appropriate.

### Data Flow After Fix

```
User B attempts clipboard paste
    ↓
authorizeUserToSession() → session has OrganizationID ✓
    ↓
authorizeOrgMember() → User B is org member ✓
    ↓
authorizeUserToResource(ProjectID, ResourceProject, ActionGet)
    ↓
getAuthzConfigs() → finds User B's access grant with role binding
    ↓
evaluate(ResourceProject, ActionGet, configs) → role now includes "Project" → SUCCESS ✓
```

## Alternatives Considered

1. **Add Session resource type** - More complex, requires new resource type and migrations
2. **Change session auth to check Application** - Would be incorrect semantically
3. **Special-case SpecTask sessions** - Adds complexity, doesn't fix the general RBAC gap

## Testing

Verified bug reproduction via browser MCP:
1. User A creates spectask, starts planning (creates session)
2. User B added to org and project with "write" role
3. User B calls clipboard API → 403 Forbidden (bug confirmed)
4. Database shows session has correct org_id/project_id
5. Database shows User B has access_grant and role_binding
6. Role config shows only ["Application", "Knowledge"], missing "Project"

After fix:
1. Same setup
2. User B calls clipboard API → 200 OK (expected)

## Migration

Existing roles need to be updated to include "Project" resource. Options:
1. Migration script to update existing role configs
2. Lazy update on role access
3. Manual update for existing orgs (if few)