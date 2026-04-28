# Implementation Tasks

- [x] Update `SessionURL` construction in `api/pkg/notification/notification_email.go` for `EventCronTriggerComplete` (line ~166) to use `/orgs/<OrganizationID>/session/<SessionID>` (no fallback — see design.md, CLAUDE.md "no fallbacks")
- [x] Apply the same change for `EventCronTriggerFailed` (line ~179)
- [x] Update existing unit tests in `api/pkg/notification/notification_email_test.go` for both `CronTriggerComplete` and `CronTriggerFailed` to assert the new `/orgs/<id>/session/<id>` URL shape
- [x] `go build ./api/pkg/notification/...` and run the unit tests (all 4 `Test_getEmailMessage_*` tests pass)
- [~] Manually verify in the inner Helix dev environment: trigger a cron task in an org and confirm the "Continue reading" link resolves (not a 404)
- [ ] Push to `feature/001953-fix-bug-continue-reading` so the Helix platform can open the PR
