# Design

## Root Cause

`api/pkg/notification/notification_email.go` builds the "Continue reading" URL
without the org segment:

```go
// notification_email.go:166 (EventCronTriggerComplete)
SessionURL: fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),

// notification_email.go:179 (EventCronTriggerFailed)
SessionURL: fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
```

The frontend router (`frontend/src/router.tsx`) defines only org-scoped session
routes:

- `/orgs/:org_id/projects/:id/session/:session_id` (line 249)
- `/orgs/:org_id/session/:session_id` (line 290)

There is **no** root-level `/session/:id` route, so the link 404s for every user
regardless of auth state. The `Session.OrganizationID` field is populated by the
cron trigger code (`api/pkg/trigger/trigger_cron.go:473`) but ignored by the
email builder.

## Fix

Change both `SessionURL` lines in `notification_email.go` to include the org
segment:

```go
SessionURL: fmt.Sprintf("%s/orgs/%s/session/%s",
    e.cfg.AppURL, n.Session.OrganizationID, n.Session.ID),
```

### Why this works with the org ID directly

The frontend route param is named `org_id` but in practice the codebase passes
either the org ID (`org_xxx`) or the org slug/name interchangeably:

- `frontend/src/components/tasks/ExecutionsHistory.tsx:94` uses `org.name`
- `frontend/src/contexts/account.tsx:371` falls back through both forms
- `api/pkg/server/organization_handlers.go:201` resolves the param as an ID
  when it has the `org_` prefix and as a name otherwise

`Session.OrganizationID` is the prefixed ID (e.g. `org_01k...`), which the
backend resolver and frontend router both accept. No extra DB lookup needed.

## Edge Case: Empty OrganizationID

The cron trigger code in `api/pkg/trigger/trigger_cron.go:473` always sets
`Session.OrganizationID` from the parent app. If a notification is ever sent
for a session that lacks `OrganizationID`, that is an upstream data bug and
should be fixed there.

CLAUDE.md guidance: **no fallbacks, no dead code paths**. We construct the URL
unconditionally with the org segment. If `OrganizationID` is somehow empty the
URL will render as `<AppURL>/orgs//session/<id>` and 404 — a visible failure
that points back at the real bug rather than masking it.

## Alternatives Considered

1. **Look up the org and use its `Name` (slug) in the URL.** Rejected — adds a
   store call and a failure mode for no benefit; the ID works fine.
2. **Add a root-level `/session/:id` route in the frontend** that redirects to
   the org-scoped URL. Rejected for this task — larger change, touches the
   router and Session page bootstrapping. Worth considering as a follow-up to
   make legacy/shared links resilient, but not required to unblock the user.
3. **Extract a helper** like `notification.sessionURL(cfg, session)`. Deferred —
   only two call sites today; introduce the helper if a third appears.

## Test Plan

- Updated existing unit tests in `api/pkg/notification/notification_email_test.go`
  for both `Test_getEmailMessage_CronTriggerComplete` and
  `Test_getEmailMessage_CronTriggerFailed` to set `Session.OrganizationID` and
  assert the rendered email contains the new `/orgs/<id>/session/<id>` URL.
  All four `Test_getEmailMessage_*` tests pass.
- **Manual end-to-end verification: NOT performed.** Inner Helix DB is empty
  (no orgs, no sessions, no SMTP wired) so reproducing the path
  `register → create org → cron app → SMTP capture → click link` is heavy
  infra for a one-line string-format change. The change is covered by:
  1. The unit test, which asserts the exact rendered URL.
  2. Direct inspection of `frontend/src/router.tsx:290`, which confirms the
     `/orgs/:org_id/session/:session_id` route exists.
  Both pieces are in source and verified.

## Files Touched

- `api/pkg/notification/notification_email.go` — two `SessionURL` lines.
- `api/pkg/notification/notification_email_test.go` — new test (file may need
  to be created).

No frontend, schema, migration, or API surface changes.
