# Default APP_URL to SERVER_URL for self-hosted instances

## Summary

Self-hosted Helix instances were generating Slack and email links pointing to `https://app.helix.ml` instead of their own URL, even when `SERVER_URL` was set correctly.

Root cause: `Notifications.AppURL` (env var `APP_URL`) had a hardcoded default of `https://app.helix.ml`. Setting `SERVER_URL` alone wasn't enough — admins had to also know about and set `APP_URL`.

This change drops the hardcoded default and falls back to `WebServer.URL` (`SERVER_URL`) when `APP_URL` is unset. Operators that want to override the public URL separately (e.g. behind a CDN) can still set `APP_URL` explicitly.

## Changes

- `api/pkg/config/config.go` — remove `default:"https://app.helix.ml"` from `Notifications.AppURL`; in `LoadServerConfig`, fall back to `WebServer.URL` when empty.

## Affected User-Visible URLs

This fixes the base URL for:
- Slack project update task links (the original report)
- Slack attention-event thread reply links
- Email notification session links

## Test Plan

- [x] `go test ./pkg/config/... ./pkg/trigger/slack/... ./pkg/notification/...` — pass
- [ ] Verify on `meta.helix.ml`: spec task links in Slack point to `meta.helix.ml`
- [ ] Verify behaviour when `APP_URL` is set explicitly — should still take precedence
