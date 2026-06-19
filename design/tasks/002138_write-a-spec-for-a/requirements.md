# Requirements: Notification Preferences Page

## Background

Helix already sends email and webhook notifications for events like cron trigger completions, failures, and org invitations. Users have no way to control which notifications they receive — it's all-or-nothing based on server config. This page lets each user opt in or out of notifications per event type.

## User Stories

**As a user, I want to enable or disable email notifications per event type** so I don't get flooded with emails I don't care about.

**As a user, I want to set a webhook URL for specific events** so I can integrate Helix notifications into my own tooling (Slack, PagerDuty, etc.) without changing the system-wide config.

**As a user, I want my notification preferences to persist** across sessions without having to reconfigure them each time.

## Acceptance Criteria

1. A "Notifications" tab appears in Account settings alongside General, Chat, and API Keys.
2. The page shows a row for each supported event type:
   - Cron trigger complete
   - Cron trigger failed
   - Org invitation (read-only informational note — always sent, cannot be disabled)
3. Each row has:
   - An email toggle (on/off switch)
   - A webhook URL text field (empty = disabled)
4. Saving updates preferences via PUT `/api/v1/users/me/notification-settings`.
5. On page load, current preferences are fetched from GET `/api/v1/users/me/notification-settings` and pre-filled.
6. A "Save" button commits all changes; a success snackbar confirms.
7. Validation: webhook URL must be empty or a valid `http://` or `https://` URL.
