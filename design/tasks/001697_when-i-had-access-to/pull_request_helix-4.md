# Fix 403 error when resuming Human Desktop on shared projects

## Summary
Users with access to a shared project were getting a 403 error when trying to resume the Human Desktop (exploratory session). The root cause was that `session.ProjectID` was not being set when creating exploratory sessions - only `session.Metadata.ProjectID` was set. The authorization check in `authorizeUserToSession()` reads `session.ProjectID` for RBAC, so it was failing with "not authorized to access session without a project".

## Changes
- Set `session.ProjectID = projectID` when creating exploratory sessions in `startExploratorySession()`

## Testing
- Verified by tracing through authorization logic in `authz.go`
- With this fix, `authorizeUserToSession()` can now properly call `authorizeUserToResource()` with the project ID for RBAC evaluation
