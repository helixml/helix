# Design: Make Project RBAC Better

## Overview

Two improvements to the project access grant flow:
1. Auto-add users to the organisation when granting them project access
2. Filter the logged-in user from the "Add User" dropdown

## Current Architecture

- **Backend**: `createProjectAccessGrant` in `api/pkg/server/project_access_grant_handlers.go` (line 104) creates an `AccessGrant`. It resolves the user by email or ID but does **not** check or create org membership.
- **Frontend**: `AccessManagement.tsx` (line 94-105) populates the user dropdown from `organization.memberships` only. Users not in the org cannot be selected.
- **Org membership**: `addOrganizationMember` in `api/pkg/server/organization_member_handlers.go` (line 53) requires org owner permissions and creates an `OrganizationMembership`.

## Design

### Change 1: Auto-add to organisation

**Backend** (`project_access_grant_handlers.go`, `createProjectAccessGrant`):

After resolving the target user (line 174), check if they are already an org member:

```
memberships, err := store.ListOrganizationMemberships(ctx, {OrgID, UserID})
```

If no membership exists:
1. Verify the acting user is an org owner (call `authorizeOrgOwner`). If not, return 403 with message: "User is not an organisation member. Only org owners can auto-add users to the organisation."
2. Call `store.CreateOrganizationMembership` with role `member`.
3. Set a flag `addedToOrganization = true`.

**Response**: Wrap the response in a new type or add a field. Cleanest option — create a new response struct:

```go
type CreateAccessGrantResponse struct {
    *AccessGrant
    AddedToOrganization bool `json:"added_to_organization"`
}
```

This is backwards-compatible (the new field defaults to `false` for existing clients).

**Frontend** (`AccessManagement.tsx`):

- In the `onCreateGrant` callback handler, check if the response includes `added_to_organization: true`.
- If so, show a snackbar/notification: "{user name} was also added to the organisation."
- Change the user dropdown to accept either a selection from org members **or** a typed email for users not yet in the org. Use a MUI `Autocomplete` with `freeSolo` to allow typing an email directly. The dropdown options remain org members; free-form input allows referencing any registered user by email.
- **Make the non-member state obvious in the UI**: When the user types an email that doesn't match any org member, show an inline info banner below the input (e.g. MUI `Alert` severity `info`): _"This user is not in your organisation. Adding them to this project will also add them to the organisation as a member."_ This appears as soon as the typed value is a valid email that doesn't match a dropdown option, **before** the user clicks "Add" — so the side effect is clearly communicated upfront, not just after the fact.
- Update the helper text from "Can't see the user? Invite them to your org" to: "You can also type an email address to add someone who isn't in your organisation yet."

### Change 2: Filter logged-in user from dropdown

**Frontend only** (`AccessManagement.tsx`, line 94-105):

Add a filter to the `members` useMemo:

```typescript
const members = useMemo(() => {
  if (!organization?.memberships) return [];
  return organization.memberships
    .filter(membership => membership.user_id !== currentUser?.id)  // NEW
    .map(membership => {
      const user = (membership.user || {}) as any;
      return {
        id: membership.user_id,
        name: user.full_name || 'Unknown',
        email: user.email || 'No email'
      };
    });
}, [organization, currentUser?.id]);  // add currentUser?.id to deps
```

Also filter out users who already have an access grant on this project, to avoid duplicate grants:

```typescript
const existingUserIds = new Set(accessGrants.map(g => g.user_id).filter(Boolean));
// ...then in the filter chain:
.filter(membership => !existingUserIds.has(membership.user_id))
```

## Key Decisions

1. **Auto-add requires org owner permissions** — Only org owners can add members to an org. If the acting user is a project owner but not an org owner, they get a clear error. This preserves the org-level permission model.

2. **Response struct extension over separate endpoint** — Adding `added_to_organization` to the create-grant response is simpler and more atomic than requiring the frontend to make two API calls. It keeps the operation transactional.

3. **Autocomplete with freeSolo over separate "invite" flow** — Replacing the `Select` with an `Autocomplete` that allows typing an email is minimal UI change while enabling the auto-add flow. No need for a separate invite dialog.

4. **Frontend-only for self-filtering** — No backend change needed for requirement 2. The backend already prevents duplicate grants, so this is purely a UX improvement.

## Files to Modify

| File | Change |
|------|--------|
| `api/pkg/server/project_access_grant_handlers.go` | Check org membership, auto-add if missing, wrap response |
| `api/pkg/types/authz.go` | Add `CreateAccessGrantResponse` struct |
| `frontend/src/components/app/AccessManagement.tsx` | Filter self from dropdown, switch to Autocomplete, show snackbar on auto-add |
