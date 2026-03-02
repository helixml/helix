# Implementation Tasks

## Investigation

- [ ] Verify hypothesis by logging `session.OrganizationID` and `session.ProjectID` in `authorizeUserToSession()` when 403 occurs
- [ ] Confirm spectask has `OrganizationID` and `ProjectID` populated correctly

## Core Fix

- [ ] In `api/pkg/server/session_handlers.go` `startChatSessionHandler()`: when creating a new session with `SpecTaskID` in metadata, look up the spectask and copy `OrganizationID`/`ProjectID` to the session if not already set
- [ ] In `api/pkg/server/session_handlers.go` `resumeSession()`: verify session has org/project IDs, backfill from spectask if missing

## Data Migration

- [ ] Write a migration or startup routine to backfill `OrganizationID`/`ProjectID` on existing sessions that have a `SpecTaskID` but empty org/project fields

## Testing

- [ ] Test: User A creates spectask, User B (org member) can resume session
- [ ] Test: User A creates spectask, User B (org member) can paste to clipboard
- [ ] Test: User C (no project access) still gets 403 on all operations
- [ ] Test: Existing direct-owner access still works (no regression)