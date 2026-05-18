# Design: Cascade Delete User Memberships and Confirm Before Admin Delete

## Current State

- `DELETE /api/v1/admin/users/{id}` → `adminDeleteUser`
  (`api/pkg/server/handlers.go:956-1012`) → `store.DeleteUser`
  (`api/pkg/store/store_users.go:179-205`).
- `store.DeleteUser` deletes the `users` row, the user's API keys and the
  user's metadata. It does **not** explicitly touch organisation or team
  memberships.
- `team_memberships.user_id` has an FK to `users.id` with `ON DELETE
  CASCADE` (`api/pkg/store/postgres.go:~253`), so team rows are removed
  by the database. In practice this cascade exists but is brittle — it is
  not exercised in tests, and any DB lacking the constraint (older
  deployments, sqlite-backed tests) leaves orphans.
- `organization_memberships` has **no** FK cascade on `user_id`. Rows are
  only cleaned up by `store.DeleteOrganizationMembership`
  (`api/pkg/store/organization_membership.go:114-136`), which the admin
  delete path never calls. This is the primary cause of "unknown user"
  appearing in org member lists.
- Frontend: `UsersTable.tsx` already opens a `DeleteUserDialog`
  (`frontend/src/components/dashboard/DeleteUserDialog.tsx`) that requires
  the admin to type `delete`. Its warning text mentions only "API keys
  and user settings".

## Design Decisions

### D1 — Do the membership cleanup in the store layer, in one transaction
Add membership cleanup to `store.DeleteUser` rather than to the HTTP
handler so that every caller of `DeleteUser` (admin endpoint, future
self-service flow, tests, scripts) gets consistent behaviour. Wrap the
whole delete in a single `gorm` transaction so a partial failure rolls
back and the user stays consistent with their memberships.

### D2 — Explicit deletes, do not rely solely on FK cascade
Even though the FK cascade exists for `team_memberships`, perform an
explicit `DELETE FROM team_memberships WHERE user_id = ?` inside the
transaction. Reasons:
- Mirrors `DeleteOrganizationMembership`'s existing two-step pattern.
- Works on backends/test setups where the constraint is missing.
- Makes the intent visible in code instead of hiding it in a migration.

### D3 — Order of operations
Within the transaction:
1. Delete `team_memberships WHERE user_id = ?`
2. Delete `organization_memberships WHERE user_id = ?`
3. Delete API keys (existing)
4. Delete user metadata (existing)
5. Delete the `users` row (existing)

Teams first, then org memberships — matches the existing pattern in
`DeleteOrganizationMembership` (teams are scoped under orgs).

### D4 — Frontend warning text only, no new dialog
Reuse `DeleteUserDialog`. Update the `<Alert severity="error">` body to
explicitly list the cascading effect: "removed from all teams and
organisations they belong to" alongside the existing API keys/settings
text. Keep the type-`delete`-to-confirm interaction unchanged — it is
already the warning-and-confirmation flow the requirement asks for.

If the dialog is *not* currently wired up in the action menu for some
build of the project, wire it up; do not invent a second confirmation
mechanism.

### D5 — Out of scope: ownership transfer
If the deleted user is the sole `owner` of an organisation, that
organisation is left with no owner. This matches today's behaviour for
manually-removed org memberships and is out of scope here. Track as a
follow-up.

## Affected Files

Backend
- `api/pkg/store/store_users.go` — modify `DeleteUser` to wrap existing
  deletes in a transaction and add membership cleanup.
- `api/pkg/store/store_users_test.go` (or equivalent) — add a test that
  creates a user with team + org memberships and asserts the rows are
  gone after `DeleteUser`.
- `api/pkg/server/handlers.go` — no behavioural change; verify the
  handler surfaces store errors unchanged.

Frontend
- `frontend/src/components/dashboard/DeleteUserDialog.tsx` — update
  warning copy to mention teams + organisations.
- `frontend/src/components/dashboard/UsersTable.tsx` — confirm the
  "Delete user" menu item opens `DeleteUserDialog`; no logic change
  expected.

## Risks and Notes for Implementer

- The existing FK cascade on `team_memberships` may delete rows
  *before* the explicit `DELETE` runs. That is fine — the explicit
  statement becomes a no-op, not an error. Do not add `RETURNING` or
  rely on the affected-row count.
- `DeleteOrganizationMembership` also cascades into team memberships
  scoped to that org. The new path bypasses it (we delete all team
  memberships in step 1 anyway), so calling it per-org is unnecessary
  and would add N round-trips.
- Keep the snackbar success/error flow in
  `DeleteUserDialog` as-is; the dialog already covers the "warning
  notification" half of US-2.
- This codebase has both a `team_membership.go` and an
  `organization_membership.go` store file with parallel patterns —
  follow their style (`s.gdb.WithContext(ctx).Where(...).Delete(...)`).

## Learnings Worth Carrying Forward

- The cascade pattern in `DeleteOrganizationMembership` (transaction +
  delete child rows then parent) is the local idiom. Reuse it.
- FK cascades exist for some membership tables and not others; do not
  assume symmetry. Always grep `postgres.go` for the relevant
  constraint before relying on the DB to clean up.
- `DeleteUserDialog` is the canonical destructive-action dialog in the
  admin area — copy its "type the word to confirm" pattern for other
  destructive admin actions rather than inventing a new component.
