# Implementation Tasks

- [x] Fix bell badge count: change `badgeContent={hasNew ? unreadCount : totalCount}` to `badgeContent={totalCount}` in `GlobalNotifications.tsx:495`
- [x] Add `AssigneeName string` field to `AttentionEvent` Go struct in `api/pkg/types/attention_event.go` (denormalized, matching existing `ProjectName`/`SpecTaskName` pattern)
- [x] Populate `AssigneeName` at event creation time in `api/pkg/services/attention_service.go` — look up spec task's assignee and resolve to user's full_name
- [~] Add `assignee_name?: string` to `AttentionEvent` TypeScript interface in `frontend/src/hooks/useAttentionEvents.ts`
- [ ] Add assignee avatar (MUI `Avatar`, 16px, initials) to the left of the timestamp in notification items when `filterMine=false` in `GlobalNotifications.tsx`
