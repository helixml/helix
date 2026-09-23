# Search admin organisations by owner email

## Summary
Extend the admin organisations search to match the organisation owner's email address as well as the organisation name. This lets administrators find the correct organisation and grant credits when they know the owner's email but not the organisation name.

Update the search field label and generated API documentation to advertise the expanded search behavior.

## Testing
Ran `go test ./pkg/server -run 'TestOrganizationSearchName|TestAdminListOrganizations' -count=1`; all focused server tests passed, including the new case-insensitive owner-email regression test.

Ran `git diff --check`; no whitespace errors were found. Frontend TypeScript compilation could not run because frontend dependencies are not installed in the checkout (`tsc: not found`); the frontend change is limited to the search field label.
