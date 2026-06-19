# Implementation Tasks: Notification Preferences Page

- [ ] Add `UserNotificationPreference` and `UserNotificationSettings` types to `api/pkg/types/authz.go`
- [ ] Add `NotificationSettings UserNotificationSettings` field to `UserMeta` struct in `api/pkg/types/authz.go`
- [ ] Add `getUserNotificationSettings` and `updateUserNotificationSettings` handlers in `api/pkg/server/user_handlers.go` with swagger annotations
- [ ] Register the two new routes in the API server router
- [ ] Run `./stack update_openapi` to regenerate the TypeScript API client
- [ ] Add `useGetUserNotificationSettings` and `useUpdateUserNotificationSettings` hooks in `frontend/src/services/userService.ts`
- [ ] Create `frontend/src/components/account/NotificationSettings.tsx` with toggle + webhook URL fields per event type
- [ ] Add `notifications` tab to `Account.tsx`
- [ ] Add Notifications item to `AccountSidebar.tsx`
- [ ] Wire user notification preferences into `notification.go` dispatch (respect email toggle and per-user webhook URL)
- [ ] Validate webhook URL (must be empty or valid http/https) in both backend handler and frontend component
