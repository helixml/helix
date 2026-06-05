# Requirements: Cascade Delete User Memberships and Confirm Before Admin Delete

## Background

When a global Helix admin deletes a user from the admin Users screen, the
user record and a few related rows (API keys, user metadata) are removed, but
the user still appears in the UI of organisations, teams and projects as an
"unknown user". This is because organisation memberships, and the
relationships those memberships own, are left orphaned.

In addition, the existing `DeleteUserDialog` confirmation only mentions
"API keys and user settings" — it does not warn the admin that the user will
also be removed from every team and organisation they belong to.

## User Stories

### US-1 — Global delete removes the user from every team and organisation
As a Helix administrator,
when I delete a user from the global admin Users page,
I want that user to be removed from every team and organisation they are part of,
so that the user no longer appears as an "unknown user" anywhere in the product.

**Acceptance criteria**

- Deleting a user from `DELETE /api/v1/admin/users/{id}` removes:
  - all rows in `team_memberships` where `user_id = deleted_user_id`
  - all rows in `organization_memberships` where `user_id = deleted_user_id`
- The deletion runs in a single transaction; if any step fails the user is
  not deleted and an error is returned.
- After deletion, the deleted user no longer appears in any team or
  organisation member list in the UI (no "unknown user" placeholders for
  this user).
- API keys and user metadata continue to be deleted as today.

### US-2 — Admin sees a warning and must confirm before delete
As a Helix administrator,
when I click "Delete user" in the admin Users action menu,
I want to see a clear warning notification describing what will be removed
and a confirmation step,
so that I do not accidentally delete a user.

**Acceptance criteria**

- The delete action in `UsersTable.tsx` opens the existing
  `DeleteUserDialog` (no inline delete).
- The dialog shows a prominent warning that lists, at minimum:
  - the user's email/username/id
  - that the user will be removed from **all teams and organisations**
    they belong to
  - that API keys and user settings will be deleted
  - that the action cannot be undone
- The submit button stays disabled until the admin types `delete` into the
  confirmation field (existing behaviour, retained).
- On success a snackbar/toast confirms the deletion; on failure a snackbar
  shows the error and the dialog stays open.

## Out of Scope

- Soft-delete / anonymisation / GDPR-style retention. Deletion remains a
  hard delete.
- Cleanup of projects, spec tasks, sessions and other resources owned by
  the deleted user. Those keep their existing behaviour (orphaned
  `user_id`). The fix here is limited to membership rows, which is what
  causes the "unknown user" appearance in team/organisation member lists.
- Transferring organisation ownership when the deleted user is the sole
  owner of an organisation. Out of scope; handle as a follow-up.
- Changes to the self-service "delete my account" flow (if any). Only the
  admin path is in scope.
