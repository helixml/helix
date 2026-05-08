# Improve project RBAC: auto-add to org and filter self from dropdown

## Summary
Two UX improvements to project access management:
1. When adding a user to a project who isn't an org member, automatically add them to the org and notify the user
2. Filter the logged-in user and users with existing grants from the "Add User" dropdown

## Changes

### Backend
- Added `CreateAccessGrantResponse` type wrapping `AccessGrant` with `added_to_organization` flag (`api/pkg/types/authz.go`)
- In `createProjectAccessGrant`, check org membership before creating grant. If user is not an org member and the acting user is an org owner, auto-create org membership with `member` role (`api/pkg/server/project_access_grant_handlers.go`)
- Non-org-owners get a 403 with a clear error message when trying to add non-org-members
- Regenerated OpenAPI spec

### Frontend
- Replaced `Select` dropdown with MUI `Autocomplete` (`freeSolo`) allowing typed email input for non-org members (`AccessManagement.tsx`)
- Show inline `Alert` when typed email doesn't match any org member: "This user is not in your organisation. Adding them to this project will also add them to the organisation as a member."
- Show `Snackbar` notification after successful grant creation if the user was auto-added to the org
- Filter current user and users with existing grants from the dropdown
- Updated prop types in `ProjectMembersBar.tsx` to match new response type

## Screenshots

![Add User dialog with typed email and info alert](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001898_make-project-rbac-better/screenshots/08-add-user-autocomplete-final.png)

![Manage Access showing owner row](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001898_make-project-rbac-better/screenshots/05-manage-access-dialog.png)
