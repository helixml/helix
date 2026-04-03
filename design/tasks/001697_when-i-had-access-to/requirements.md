# Requirements: 403 Error When Resuming Shared Project's Human Desktop

## Problem Statement

When a user has access to another user's project and tries to resume the Human Desktop (exploratory session), they get a 403 "Failed to resume Human Desktop" error.

## User Stories

### Primary User Story
As a team member with access to a shared project, I want to resume the Human Desktop session so that I can continue exploring the codebase.

**Current behavior:** 403 Forbidden error
**Expected behavior:** Session resumes successfully

## Root Cause Analysis

The bug is in `session_handlers.go:resumeSession()`. The authorization flow has a field mismatch:

1. **Authorization check (line 1827):** Uses `authorizeUserToSession()` which checks `session.ProjectID`
2. **Session loading (line 1874-1877):** For exploratory sessions, loads project from `session.Metadata.ProjectID`

If `session.ProjectID` is empty but `session.Metadata.ProjectID` is set (which happens for exploratory sessions), the auth check fails at line 329-330 in `authz.go`:
```go
if session.ProjectID == "" {
    return fmt.Errorf("not authorized to access session without a project")
}
```

## Acceptance Criteria

- [ ] Users with project access can resume exploratory sessions on shared projects
- [ ] Proper 403 still returned for users without project access
- [ ] No regression for session owners resuming their own sessions
- [ ] No regression for spec task sessions (which use `session.Metadata.SpecTaskID`)
