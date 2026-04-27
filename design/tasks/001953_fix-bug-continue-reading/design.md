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

If a cron trigger were ever fired against a session without an `OrganizationID`
(personal session), the new URL would render as `<AppURL>/orgs//session/<id>`
which would also 404. In practice the cron trigger path always sets
`OrganizationID` from the parent app, so this should not happen — but to be
safe, fall back to the previous `/session/<id>` shape when `OrganizationID` is
empty. This preserves existing (broken) behaviour for the edge case rather than
making it worse, and surfaces the missing personal-session route as a separate
follow-up.

```go
sessionURL := fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID)
if n.Session.OrganizationID != "" {
    sessionURL = fmt.Sprintf("%s/orgs/%s/session/%s",
        e.cfg.AppURL, n.Session.OrganizationID, n.Session.ID)
}
```

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

- New unit test in `api/pkg/notification/notification_email_test.go` (create if
  it does not exist) asserts that `getEmailMessage` for `EventCronTriggerComplete`
  and `EventCronTriggerFailed` produces a `SessionURL` containing
  `/orgs/<org_id>/session/<session_id>` when `Session.OrganizationID` is set.
- Manual verification in the inner Helix dev environment: trigger a cron task
  in an org, capture the rendered email body, and click the link.

## Files Touched

- `api/pkg/notification/notification_email.go` — two `SessionURL` lines.
- `api/pkg/notification/notification_email_test.go` — new test (file may need
  to be created).

No frontend, schema, migration, or API surface changes.
