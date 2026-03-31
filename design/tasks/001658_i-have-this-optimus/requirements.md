# Requirements: Fix Spectask URLs in Slack Notifications

## Problem

Slack notifications from the Optimus agent include links to spectask detail pages using the wrong base URL (e.g. `https://app.helix.ml/orgs/helix/projects/.../tasks/...`) even when `SERVER_URL=https://meta.helix.ml` is set in `.env`.

The fix manually replacing `app` → `meta` in the URL confirms the correct path structure — only the base URL is wrong.

## Root Cause

There are **two separate config fields** that both represent "the base URL of this Helix instance":

| Config field | Env var | Default |
|---|---|---|
| `WebServer.URL` | `SERVER_URL` | (empty) |
| `Notifications.AppURL` | `APP_URL` | `https://app.helix.ml` |

The Slack bot (`api/pkg/trigger/slack/slack_project_updates.go`) uses `Notifications.AppURL` (`APP_URL`), not `SERVER_URL`. Since `APP_URL` defaults to `https://app.helix.ml` and is not documented as needing to be set, self-hosted installs that only set `SERVER_URL` get wrong URLs in Slack messages.

## User Stories

- As a self-hosted Helix admin, when I set `SERVER_URL=https://my.instance.com`, all spectask links in Slack notifications should point to `https://my.instance.com/...` without requiring an additional env var.

## Acceptance Criteria

- [ ] Slack spectask links use the correct base URL when only `SERVER_URL` is set
- [ ] No regression for installs that set `APP_URL` explicitly (it should still be honoured if set)
- [ ] The `APP_URL` env var is either removed or documented clearly as an override
- [ ] PR footer spectask links (from `git_http_server.go` and `spec_task_workflow_handlers.go`) continue to work correctly (they already use `SERVER_URL`)
