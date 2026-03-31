# Design: Fix Spectask URLs in Slack Notifications

## Key Files

- `api/pkg/config/config.go:223` — `Notifications.AppURL` field (`APP_URL`, defaults to `https://app.helix.ml`)
- `api/pkg/config/config.go:448` — `WebServer.URL` field (`SERVER_URL`)
- `api/pkg/trigger/slack/slack_project_updates.go:119,154,241,272` — uses `s.cfg.Notifications.AppURL` as base URL for task links
- `api/pkg/trigger/slack/slack_project_updates.go:354` — `buildTaskLink()` helper

## Approach

**Remove `APP_URL` and unify on `SERVER_URL`.**

The `Notifications.AppURL` field is redundant — `WebServer.URL` (`SERVER_URL`) already captures the public URL of the instance. Having two env vars for the same concept is the bug.

Change the Slack bot to use `cfg.WebServer.URL` (already available on the server struct) instead of `cfg.Notifications.AppURL`.

If `APP_URL` has any unique uses beyond the Slack bot, keep it but make it fall back to `SERVER_URL` when unset (instead of hard-coding `https://app.helix.ml`):

```go
// In config initialisation / after envconfig.Process():
if cfg.Notifications.AppURL == "" {
    cfg.Notifications.AppURL = cfg.WebServer.URL
}
```

But simply removing `APP_URL` from `Notifications` and wiring `buildTaskLink` directly to `cfg.WebServer.URL` is cleaner — fewer moving parts.

## Decision

Remove `Notifications.AppURL`. Pass `cfg.WebServer.URL` wherever task URLs are built in the Slack trigger code. Update tests that reference `APP_URL` or the `https://app.helix.ml` default.

## Workaround (immediate)

Until the fix is deployed, add `APP_URL=https://meta.helix.ml` to `.env`.

## Patterns Found

- PR footer links already correctly use `s.Cfg.WebServer.URL` — Slack was the only outlier.
- `buildTaskLink` in `slack_project_updates.go` accepts a `baseURL` parameter; the fix is just passing the right value into it.
- There may be test assertions using the `https://app.helix.ml` default that need updating.
