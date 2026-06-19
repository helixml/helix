# Design: Notification Preferences Page

## Overview

Add a new "Notifications" tab to the Account settings page. User preferences are stored in a new `NotificationSettings` JSONB column on the existing `UserMeta` table, following the identical pattern used by `ChatSettings`.

## Backend

### New type (`api/pkg/types/authz.go`)

```go
type UserNotificationPreference struct {
    EmailEnabled bool   `json:"email_enabled"`
    WebhookURL   string `json:"webhook_url,omitempty"`
}

type UserNotificationSettings struct {
    CronTriggerComplete *UserNotificationPreference `json:"cron_trigger_complete,omitempty"`
    CronTriggerFailed   *UserNotificationPreference `json:"cron_trigger_failed,omitempty"`
}
```

Add `NotificationSettings UserNotificationSettings` to `UserMeta` with `gorm:"type:jsonb;serializer:json"`.

### New API endpoints (`api/pkg/server/user_handlers.go`)

- `GET  /api/v1/users/me/notification-settings` → returns `UserNotificationSettings`
- `PUT  /api/v1/users/me/notification-settings` → accepts and persists `UserNotificationSettings`

Both follow the exact same pattern as `getUserChatSettings` / `updateUserChatSettings`.

Add swagger annotations and run `./stack update_openapi` to regenerate the TypeScript client.

### Wire into notification dispatch (`api/pkg/notification/notification.go`)

Before sending, look up the session owner's `NotificationSettings`. For the matching event:
- If `EmailEnabled` is false and no system-level override, skip the email send.
- If `WebhookURL` is non-empty, use it as `CallbackURL` instead of (or in addition to) any app-level callback.

## Frontend

### New component `frontend/src/components/account/NotificationSettings.tsx`

Renders a settings section with one row per event type. Each row has:
- `<Typography>` label (event name + description)
- `<Switch>` for email toggle
- `<TextField>` for webhook URL

Backed by `useGetUserNotificationSettings` and `useUpdateUserNotificationSettings` hooks in `userService.ts`, following the same React Query pattern as `useGetUserChatSettings`.

### Account page (`frontend/src/pages/Account.tsx`)

Add `{tab === 'notifications' && <NotificationSettings />}`.

### AccountSidebar (`frontend/src/components/account/AccountSidebar.tsx`)

Add a `notifications` item with `NotificationsIcon` from MUI.

## Key Decisions

- **Org invitation is always sent**: This is a transactional notification (you can't join an org if you miss the link), so it's excluded from the opt-out matrix. Show a read-only note on the page.
- **Per-event granularity**: Two event types for now (`CronTriggerComplete`, `CronTriggerFailed`). Adding more later means extending the struct and adding rows to the UI — no architectural change needed.
- **Default behaviour unchanged**: `nil` preference entries preserve today's behaviour (notifications sent based on server config). Explicit `false` disables.

## Migration

No new table needed. Add the column via GORM AutoMigrate on next startup.
