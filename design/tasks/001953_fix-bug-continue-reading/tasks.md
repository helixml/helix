# Implementation Tasks

- [~] Update `SessionURL` construction in `api/pkg/notification/notification_email.go` for `EventCronTriggerComplete` (line ~166) to use `/orgs/<OrganizationID>/session/<SessionID>` when `OrganizationID` is set, falling back to `/session/<SessionID>` otherwise
- [ ] Apply the same change for `EventCronTriggerFailed` (line ~179)
- [ ] Add a unit test in `api/pkg/notification/notification_email_test.go` covering both events: with and without `Session.OrganizationID` set, asserting the generated URL shape
- [ ] `go build ./api/pkg/notification/...` and run the new unit test
- [ ] Manually verify in the inner Helix dev environment: create a cron-trigger app in an org, run it, open the resulting email, and confirm the "Continue reading" link opens the session page (not a 404)
- [ ] Open a PR against `helixml/helix` referencing task 001953; include before/after of the rendered link and note that personal-session routing remains a separate follow-up
