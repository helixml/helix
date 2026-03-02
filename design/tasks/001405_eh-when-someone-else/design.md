# Design: Cross-User SpectTask Authorization Fix

## Overview

Fix authorization failures when org members (not the session owner) interact with spectask sessions.

## Current Behavior

```
User A creates spectask → Session created with Owner=UserA
User B (org member with project RBAC) → 403 on clipboard, 401 on resume
```

## Root Cause

The `authorizeUserToSession()` function has two paths:

```go
// Path 1: No OrganizationID - owner check only (FAILING PATH)
if session.OrganizationID == "" {
    if user.ID == session.Owner { return nil }
    return error  // ← User B fails here
}

// Path 2: Has OrganizationID - RBAC check (SHOULD WORK)
return authorizeUserToResource(session.OrganizationID, session.ProjectID, ...)
```

Sessions are falling into Path 1 because `OrganizationID` is not being propagated from the spectask to the session.

## Solution

Ensure sessions created for spectasks inherit `OrganizationID` and `ProjectID` from the parent spectask.

### Code Changes

**Location:** `api/pkg/server/session_handlers.go` - `startChatSessionHandler`

When creating a new session with a `SpecTaskID` in the request context, look up the spectask and inherit its org/project:

```go
// If this is a spectask session and org/project not set, inherit from spectask
if session.Metadata.SpecTaskID != "" && session.OrganizationID == "" {
    specTask, err := s.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
    if err == nil && specTask != nil {
        session.OrganizationID = specTask.OrganizationID
        session.ProjectID = specTask.ProjectID
    }
}
```

### Data Flow After Fix

```
SpectTask (UserID, ProjectID, OrganizationID)
    ↓
Session (Owner=UserID, ProjectID, OrganizationID)  ← NOW POPULATED
    ↓
authorizeUserToSession() → Path 2 (RBAC) → authorizeUserToResource(ProjectID) → SUCCESS
```

## Alternatives Considered

1. **Check spectask ownership in each endpoint** - Too invasive, duplicates authz logic
2. **Add spectask ID to session and check in authz** - More complex, session already has the fields

## Testing

1. Create spectask as User A
2. Start session for spectask as User A
3. As User B (org member with project access):
   - Resume session → Should succeed
   - Paste to clipboard → Should succeed
   - Screenshot → Should succeed
4. As User C (no project access):
   - All operations → Should fail with 403

## Migration

Existing sessions with empty `OrganizationID` will need backfill if their spectask has org/project set. This can be done via a one-time migration or lazy update on session access.