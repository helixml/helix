# Implementation Tasks: Cascade Delete User Memberships and Confirm Before Admin Delete

- [ ] In `api/pkg/store/store_users.go`, refactor `DeleteUser` to run inside a single `gorm` transaction
- [ ] Inside that transaction, add `DELETE FROM team_memberships WHERE user_id = ?` before the existing user delete
- [ ] Inside that transaction, add `DELETE FROM organization_memberships WHERE user_id = ?` before the existing user delete
- [ ] Keep existing deletion of API keys and user metadata; ensure all five steps share the transaction and roll back together on error
- [ ] Add a store-level test that seeds a user with at least one team membership and one organisation membership, calls `DeleteUser`, and asserts both membership tables no longer contain that `user_id`
- [ ] Verify `adminDeleteUser` in `api/pkg/server/handlers.go` still returns the store error untouched (no handler change expected)
- [ ] Update `frontend/src/components/dashboard/DeleteUserDialog.tsx` warning alert copy to state the user will be removed from all teams and organisations they belong to, in addition to API keys and user settings
- [ ] Confirm `frontend/src/components/dashboard/UsersTable.tsx` "Delete user" menu item opens `DeleteUserDialog` (wire it up if it does not) and that the confirmation-by-typing-`delete` interaction is preserved
- [ ] Manually verify in the running app: create a user, add to a team and an organisation, delete from admin, confirm the user disappears from the team and organisation member lists with no "unknown user" placeholder
- [ ] Run backend tests (`go test ./api/...`) and frontend type-check / lint to confirm no regressions
