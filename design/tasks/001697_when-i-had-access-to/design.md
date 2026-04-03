# Design: Fix 403 on Shared Project Human Desktop Resume

## Architecture Context

**Desktop Session Types:**
- **Spec task sessions:** `session.Metadata.SpecTaskID` set, project loaded via spec task
- **Exploratory sessions (Human Desktop):** `session.Metadata.ProjectID` set, no spec task

**Authorization Flow:**
```
resumeSession()
  ├─ getSession()
  ├─ authorizeUserToSession()  ← checks session.ProjectID (empty for exploratory!)
  ├─ load agent.ProjectID from session.Metadata.ProjectID
  └─ GetProject() / resume agent
```

## The Bug

`authorizeUserToSession()` reads `session.ProjectID`, but for exploratory sessions this field is empty. The project ID lives in `session.Metadata.ProjectID` instead.

At `authz.go:329-330`:
```go
if session.ProjectID == "" {
    return fmt.Errorf("not authorized to access session without a project")
}
```

This fails before RBAC can check if the user has project access.

## Solution

**Option A (chosen): Backfill `session.ProjectID` for exploratory sessions**

When creating/saving exploratory sessions, ensure `session.ProjectID = session.Metadata.ProjectID`. This makes authorization work without changing the auth logic.

**Option B: Modify `authorizeUserToSession()` to check metadata**

Add fallback: if `session.ProjectID == ""` but `session.Metadata.ProjectID != ""`, use the metadata field. More defensive but requires changing auth code.

## Decision: Option A

Keeping auth logic simple. The root cause is inconsistent data—fix the data, not the auth code.

## Key Files

| File | Change |
|------|--------|
| `api/pkg/server/project_handlers.go` | Set `session.ProjectID` when creating exploratory sessions (line 1479) |

## Implementation Notes

- The fix was a one-line change: added `ProjectID: projectID` to the session struct in `startExploratorySession()`
- No migration needed - exploratory sessions are ephemeral and get recreated; users just need to start a new session
- The authorization flow is: `authorizeUserToSession()` → checks `session.ProjectID` → calls `authorizeUserToResource()` which does RBAC lookup
- With the fix, users with project RBAC access can now resume exploratory sessions on shared projects
