# Requirements

## Bug Summary

The "Continue reading" link in scheduled-task / cron-trigger emails (e.g. the
"Daily Weather Update" task) points at `https://app.helix.ml/session/<session_id>`.
That path does not exist in the frontend router, so users land on a 404 page even
when they are logged in and have access to the organisation that owns the session.

## User Story

**As an** organisation user who has subscribed to a daily/scheduled Helix task,
**when** I receive the result email and click "Continue reading",
**I want** to land on the correct session page inside my organisation,
**so that** I can review the full output and continue the conversation.

## Acceptance Criteria

1. Clicking "Continue reading" in a `EventCronTriggerComplete` email opens the
   session page successfully (no 404) when the user is logged in and has access
   to the organisation that owns the session.
2. Clicking the equivalent link in a `EventCronTriggerFailed` email behaves the
   same way.
3. The link works for sessions that belong to an organisation. URL shape:
   `<AppURL>/orgs/<org>/session/<session_id>`.
4. If the session has no `OrganizationID` (personal session), the bug is
   acknowledged but treated as out of scope for this fix — see design.md.
5. A regression test (Go unit test against the email template builder) asserts
   the generated `SessionURL` includes the org segment when `OrganizationID` is
   set.

## Out of Scope

- Personal (non-org) cron trigger sessions. There is no frontend route for a
  bare `/session/:id`; fixing that requires a router change and is tracked
  separately.
- Email styling, copy, or other notification events
  (`PasswordResetRequest`, `WaitlistApproved`, etc.).
