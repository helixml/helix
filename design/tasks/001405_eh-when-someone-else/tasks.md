# Implementation Tasks

## Investigation (COMPLETED)

- [x] Verify hypothesis by reproducing bug in browser
- [x] Confirm session has `OrganizationID` and `ProjectID` populated correctly
- [x] Identify root cause: role config only includes `["Application", "Knowledge"]` resources, but session auth checks for `"Project"` resource

## Root Cause

The "write" role created during onboarding has this config:
```json
{"rules": [{"effect": "allow", "actions": ["Get", "List", "UseAction", "Create", "Update", "Delete"], "resource": ["Application", "Knowledge"]}]}
```

But `authorizeUserToSession()` calls:
```go
authorizeUserToResource(ctx, user, session.OrganizationID, session.ProjectID, types.ResourceProject, action)
```

The `"Project"` resource is not in the role's allowed resources, so authorization fails.

## Core Fix

- [ ] Update the default "write" role config to include `"Project"` resource in the allowed resources list
- [ ] Also consider adding `"Session"` resource type if we want finer-grained control
- [ ] Review other default roles (read, admin) to ensure they have appropriate project access

**Location:** Find where default roles are created during org/project setup

## Testing with Browser MCP

### Reproduce the bug (verified working)
1. Register User A (usera@test.com)
2. Create org "TestOrg" and project "TestProject" as User A
3. Create a SpecTask and start planning (creates session)
4. Register User B (userb@test.com)
5. Add User B to org as member
6. Add User B to project with "write" role
7. As User B, call clipboard API:
   ```javascript
   // In User B's browser console (isolated context)
   const sessionId = 'ses_XXX'; // from database
   const response = await fetch(`/api/v1/external-agents/${sessionId}/clipboard`, {
     method: 'POST',
     headers: { 'Content-Type': 'application/json' },
     body: JSON.stringify({ type: 'text', data: 'test from User B' })
   });
   console.log(response.status, await response.text());
   ```
8. **Expected:** 403 Forbidden (current bug)
9. **After fix:** 200 OK

### Verify the fix
1. Apply the fix to add "Project" to write role
2. Re-run step 7 above
3. **Expected:** 200 OK - clipboard set successfully
4. Also test: resume session as User B → should succeed

### Regression test
1. As User C (not in org), attempt clipboard API → should still get 403
2. As User A (owner), all operations → should still work

## Database Verification Commands

```bash
# Check session has org/project
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT id, owner, organization_id, project_id FROM sessions ORDER BY created DESC LIMIT 1;"

# Check User B's access grant
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT * FROM access_grants WHERE user_id = 'usr_XXX';"

# Check role config (THE BUG IS HERE)
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT name, config FROM roles WHERE organization_id = 'org_XXX';"
```
