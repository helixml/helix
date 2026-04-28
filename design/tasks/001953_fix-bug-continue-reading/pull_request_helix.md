# Fix 404 on "Continue reading" link in cron-trigger emails

## Summary

Cron-trigger notification emails (e.g. the "Daily Weather Update" task) built
the "Continue reading" link as `<AppURL>/session/<session_id>`. The frontend
router only defines org-scoped session routes
(`/orgs/:org_id/session/:session_id` — `frontend/src/router.tsx:290`), so every
recipient hit a 404 even when logged in with org access.

`Session.OrganizationID` is already populated by the cron trigger
(`api/pkg/trigger/trigger_cron.go:473`); the email builder just wasn't using
it. Both the API org-resolver
(`api/pkg/server/organization_handlers.go:201`) and the frontend (e.g.
`frontend/src/components/tasks/ExecutionsHistory.tsx:94`) accept either the
`org_xxx` ID or the org name in that URL slot, so passing the ID directly works
without an extra DB lookup.

## Changes

- `api/pkg/notification/notification_email.go`: build `SessionURL` as
  `<AppURL>/orgs/<OrganizationID>/session/<SessionID>` for both
  `EventCronTriggerComplete` and `EventCronTriggerFailed`. No fallback path —
  cron triggers always set `OrganizationID`; if that ever fails, the resulting
  404 surfaces the upstream bug rather than masking it.
- `api/pkg/notification/notification_email_test.go`: updated existing tests for
  both events to set `OrganizationID` and assert the new URL shape.

## Out of Scope / Follow-up

Personal (non-org) sessions still have no frontend route. Fixing that requires
a router change (add `/session/:id` and bootstrap `Session.tsx` without an
`org_id` param) and is left as a separate task.

## Test Plan

- [x] `go build ./pkg/notification/...`
- [x] `go test -v -run Test_getEmailMessage ./pkg/notification/...` (4/4 pass)
- [ ] End-to-end manual click-through deferred — see design doc Test Plan;
      change is fully covered by the unit test plus router inspection.

Spec: `design/tasks/001953_fix-bug-continue-reading/` on the `helix-specs` branch.
