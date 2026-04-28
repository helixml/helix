# Implementation Tasks

- [x] Update `SessionURL` construction in `api/pkg/notification/notification_email.go` for `EventCronTriggerComplete` (line ~166) to use `/orgs/<OrganizationID>/session/<SessionID>` (no fallback — see design.md, CLAUDE.md "no fallbacks")
- [x] Apply the same change for `EventCronTriggerFailed` (line ~179)
- [x] Update existing unit tests in `api/pkg/notification/notification_email_test.go` for both `CronTriggerComplete` and `CronTriggerFailed` to assert the new `/orgs/<id>/session/<id>` URL shape
- [x] `go build ./api/pkg/notification/...` and run the unit tests (all 4 `Test_getEmailMessage_*` tests pass)
- [x] ~~Manually verify in the inner Helix dev environment~~ — **deferred, see design.md Test Plan**: inner DB is empty (no orgs/sessions/SMTP); the change is a string-format swap covered by the unit test and verified against the existing frontend route at `router.tsx:290`
- [x] Write PR description and push to `feature/001953-fix-bug-continue-reading`
