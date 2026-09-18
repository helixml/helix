# Clear up revoke-trial feedback and move billing actions to the org screen

## Summary

The Revoke trial action on the admin Users screen was fire-and-forget with no user-visible outcome: the backend relied on the Stripe `customer.subscription.deleted` webhook to clear the wallet, so after "revoke" the trial chip stayed active after reload, and the target org was silently chosen as the user's oldest owned org. This PR fixes the feedback loop and, per design review, reorganises where billing actions live.

Backend (`api/pkg/server/admin_trial_handlers.go`):
- `DELETE /admin/users/{id}/trial-activate` now takes an `org_id` query param (required when the user owns organisations, mirroring the activate endpoint) and cancels only that org's trialing subscription.
- After a successful Stripe cancel, the cancelled wallet state is mirrored synchronously (same pattern as activate), so the admin UI reflects the revoke on the next reload without waiting for the webhook.

Frontend:
- New org admin UI: `AdminOrgsTable` migrated to the shared `SimpleTable` pattern with a single Lucide actions column, a dedicated subscription chip (Trialing/Active), and an explicit Plan cell ("Auto — from Stripe" vs a "forced" chip).
- New **Set plan** dialog replacing the terse "Set Pro / Set Free / Clear override" menu: shows the current state, offers Force Pro / Force Free with one-line explanations, and surfaces **Remove override (back to Stripe)** only when an override exists. Apply is disabled until the selection differs from the current state.
- New **AdminOrgBillingDialog** on the org screen for Activate trial (days + credits), Revoke trial (only when the wallet is trialing) and Grant credits, with in-dialog error surfacing, pending states and success snackbars; plan actions also got snackbars.
- User screen keeps Approve / Reset password / Delete user, plus the onboarding stash flow: **Activate trial** for org-less users stashes the intent for their first org, prefills from an existing stash, warns that re-activating replaces it, and offers **Clear stashed trial** so a mistaken grant can be undone before the org exists. Users who own organisations are pointed to the org screen instead of picking one inline.
- Service hooks take an optional `org_id` to serve both surfaces and invalidate both the users and admin-orgs query keys.

## Testing

- Go: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` and `go test ./pkg/server/ -run 'TestAdminRevokeTrial|TestAdminActivateTrial'` — revoke tests rewritten for explicit-org selection (cancel + wallet-mirror assertions), org-required validation, and stash clearing; all pass.
- Frontend: `tsc --noEmit`, dashboard vitest suite (21 tests), and `yarn build` all green.
- End-to-end in the inner Helix dev stack (cloud edition, Stripe disabled):
  - Stash path: seeded stashed intent → dialog → activate → chip "Pending (30d)" → "—" after reload, DB cleared, backend log confirms.
  - Stash clear: "Clear stashed trial" → snackbar, chip cleared, DB fields nulled.
  - Org billing: menu shows the correct items per wallet state; activate/credits submit and surface backend errors in the dialog ("Stripe billing must be enabled" / wallet errors) rather than failing silently.
  - Set plan dialog: current-state alert, option list, no-op guard, and the full clear-override happy path (seeded `pro` override → removed → snackbar + Plan cell back to Auto).
  - User menus verified for waitlisted, active and org-owning users.
- Not covered live: Stripe-dependent happy paths (real subscription cancel/create) — exercised only by Go unit tests against a mocked Stripe backend.
