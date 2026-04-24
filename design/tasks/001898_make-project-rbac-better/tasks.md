# Implementation Tasks

## Backend

- [ ] Add `CreateAccessGrantResponse` struct to `api/pkg/types/authz.go` with `AddedToOrganization bool` field
- [ ] In `createProjectAccessGrant` (`api/pkg/server/project_access_grant_handlers.go`), after resolving the user, check if they have an org membership via `ListOrganizationMemberships`
- [ ] If no membership exists: check acting user is org owner, then call `CreateOrganizationMembership` with role `member`
- [ ] If no membership and acting user is not org owner: return 403 with descriptive error message
- [ ] Return `CreateAccessGrantResponse` (with `added_to_organization` flag) instead of raw `AccessGrant`
- [ ] Regenerate OpenAPI spec (`./stack update_openapi`) so frontend client picks up the new response type

## Frontend

- [ ] In `AccessManagement.tsx`, filter `currentUser.id` out of the `members` list in the `useMemo` (line ~95)
- [ ] Also filter out users who already have an access grant on this project (using `accessGrants` user IDs)
- [ ] Replace the `Select` dropdown for users with MUI `Autocomplete` (`freeSolo`) — options from org members, allow typing an email for non-members
- [ ] Update `handleCreateUserGrant` to pass either the selected user ID or the typed email as `user_reference`
- [ ] After successful grant creation, check `added_to_organization` in the response; if true, show a snackbar: "{name} was also added to the organisation"
- [ ] Update the helper text below the user input to mention that typing an email will auto-add the user to the org

## Testing

- [ ] Test: add a user who IS already an org member → grant created, no auto-add, no snackbar
- [ ] Test: add a user who is NOT an org member (by email) → auto-added to org, grant created, snackbar shown
- [ ] Test: non-org-owner tries to add non-org-member → 403 error with clear message
- [ ] Test: logged-in user does not appear in the user dropdown
- [ ] Test: users who already have a grant do not appear in the dropdown
- [ ] Build frontend (`cd frontend && yarn build`) and verify no errors
- [ ] Build backend (`go build ./api/...`) and verify no errors
