# Design: Fix Spectask URLs in Slack Notifications

## Key Files

- `api/pkg/config/config.go:223` — `Notifications.AppURL` field (`APP_URL`, defaults to `https://app.helix.ml`)
- `api/pkg/config/config.go:51-58` — `LoadServerConfig()` runs `envconfig.Process` and returns
- `api/pkg/config/config.go:448-449` — `WebServer.URL` field (`SERVER_URL`)
- `api/pkg/trigger/slack/slack_project_updates.go:119,154,241` — uses `s.cfg.Notifications.AppURL`
- `api/pkg/services/attention_service.go:191-194` — also uses `s.cfg.Notifications.AppURL`
- `api/pkg/notification/notification_email.go:166,179,204` — email templates use the same field via `e.cfg.AppURL` (where `e.cfg` is `*config.Notifications`)

## Implementation Notes (discovered)

`Notifications.AppURL` is **not just used by Slack** — it's the same field that drives:
- Slack project update task links (the reported bug)
- Attention service Slack thread reply links
- Email notification session URLs

Originally I planned to remove the field entirely, but that would break email and attention-service URLs. The cleaner fix is:

**Make `APP_URL` fall back to `SERVER_URL` instead of hardcoded `https://app.helix.ml`.**

`Notifications.AppURL` and `WebServer.URL` are conceptually distinct (one might want a public URL different from the API listen URL — e.g. behind a CDN/edge), so keeping both is valid. The bug is only in the **default**: when `APP_URL` is unset, falling back to `https://app.helix.ml` is wrong for self-hosted installs.

## Approach

1. Remove the `default:"https://app.helix.ml"` tag from `Notifications.AppURL`
2. In `LoadServerConfig()`, after `envconfig.Process`, set `cfg.Notifications.AppURL = cfg.WebServer.URL` if empty
3. Existing tests for Slack (`slack_bot_test.go`) pass `https://app.helix.ml` explicitly to `buildProjectUpdateAttachment`, so they continue to work without modification.
4. Email tests construct `Notifications{AppURL: "..."}` directly — also unaffected.

## Behaviour matrix after fix

| `SERVER_URL` set | `APP_URL` set | Resulting `Notifications.AppURL` |
|---|---|---|
| `https://meta.helix.ml` | (unset) | `https://meta.helix.ml` (the fix) |
| `https://meta.helix.ml` | `https://public.helix.ml` | `https://public.helix.ml` (override still works) |
| (unset) | (unset) | empty (degraded but acceptable — `SERVER_URL` is normally required for the API to function) |

## Out of Scope

- `Janitor.AppURL` and `Stripe.AppURL` are different fields with no envconfig tag at all and are never populated programmatically — they're already broken, but unrelated to this bug. Leaving them alone.

## Workaround (immediate, no code change)

Set `APP_URL=https://meta.helix.ml` in `.env`.
