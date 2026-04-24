# Implementation Tasks

## Backend

- [x] Add `CreateAccessGrantResponse` struct to `api/pkg/types/authz.go` with `AddedToOrganization bool` field
- [x] In `createProjectAccessGrant` (`api/pkg/server/project_access_grant_handlers.go`), after resolving the user, check if they have an org membership via `ListOrganizationMemberships`
- [x] If no membership exists: check acting user is org owner, then call `CreateOrganizationMembership` with role `member`
- [x] If no membership and acting user is not org owner: return 403 with descriptive error message
- [x] Return `CreateAccessGrantResponse` (with `added_to_organization` flag) instead of raw `AccessGrant`
- [x] Regenerate OpenAPI spec (`./stack update_openapi`) so frontend client picks up the new response type

## Frontend

- [~] In `AccessManagement.tsx`, filter `currentUser.id` out of the `members` list in the `useMemo` (line ~95)
- [~] Also filter out users who already have an access grant on this project (using `accessGrants` user IDs)
- [~] Replace the `Select` dropdown for users with MUI `Autocomplete` (`freeSolo`) — options from org members, allow typing an email for non-members
- [~] Update `handleCreateUserGrant` to pass either the selected user ID or the typed email as `user_reference`
- [~] When a typed email doesn't match any org member, show an inline `Alert` (info) below the input: "This user is not in your organisation. Adding them to this project will also add them to the organisation as a member."
- [~] After successful grant creation, check `added_to_organization` in the response; if true, show a snackbar: "{name} was also added to the organisation"
- [~] Update the helper text below the user input to: "You can also type an email address to add someone who isn't in your organisation yet."

## Testing

- [ ] Test: add a user who IS already an org member → grant created, no auto-add, no snackbar
- [ ] Test: add a user who is NOT an org member (by email) → auto-added to org, grant created, snackbar shown
- [ ] Test: non-org-owner tries to add non-org-member → 403 error with clear message
- [ ] Test: logged-in user does not appear in the user dropdown
- [ ] Test: users who already have a grant do not appear in the dropdown
- [ ] Build frontend (`cd frontend && yarn build`) and verify no errors
- [ ] Build backend (`go build ./api/...`) and verify no errors
