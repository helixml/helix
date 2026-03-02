# Requirements: Cross-User SpectTask Authorization

## Problem Statement

When User B (who has RBAC access to a project via organization membership) tries to interact with a spectask session created by User A, they receive:
- **403 Forbidden** on the clipboard PASTE endpoint
- **401 Unauthorized** when trying to start/resume the spectask container

## User Stories

### US1: Team Member Can Paste to Shared SpectTask
**As** a team member with project access via RBAC  
**I want** to paste clipboard content into a spectask session created by another user  
**So that** I can collaborate on tasks without permission errors

### US2: Team Member Can Start/Resume Shared SpectTask Container
**As** a team member with project access via RBAC  
**I want** to start or resume a spectask's container that was created by another user  
**So that** I can work on shared tasks without needing the original creator

## Acceptance Criteria

1. **AC1**: Users with project-level access (via org RBAC) can use clipboard endpoints on sessions they didn't create
2. **AC2**: Users with project-level access can start/resume spectask containers they didn't create
3. **AC3**: Existing owner-based access continues to work (no regression)
4. **AC4**: Users WITHOUT project access still get 403/401 appropriately

## Root Cause Analysis

The `authorizeUserToSession()` function in `api/pkg/server/authz.go` correctly supports RBAC authorization via `authorizeUserToResource()`, but this only works when:
1. `session.OrganizationID` is set
2. `session.ProjectID` is set

**The bug**: When sessions are created for spectasks, either:
- The `OrganizationID`/`ProjectID` are not being passed from the frontend when starting the session, OR
- The session is created with empty org/project IDs, falling back to owner-only checks

The authorization flow is:
1. If `OrganizationID == ""` → Only `session.Owner == user.ID` passes (this is the failing path)
2. If `OrganizationID != ""` → RBAC via `authorizeUserToResource(session.ProjectID)` should work

## Scope

**In Scope:**
- Ensuring spectask sessions have `OrganizationID` and `ProjectID` set from the parent spectask
- Verifying the authorization flow works for all external agent endpoints

**Out of Scope:**
- Changing the RBAC model itself
- Adding new permission types