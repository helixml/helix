# Requirements: Notification Fixes

## User Stories

### 1. Correct notification sort order
As a user, I want notifications sorted by most recent activity (newest first) so I can see the latest updates at the top.

**Acceptance Criteria:**
- Notifications in the panel are ordered by `created_at DESC` of the most recent event per task
- Currently the SQL `DISTINCT ON (spec_task_id) ORDER BY spec_task_id, created_at DESC` sorts by `spec_task_id` UUID after deduplication, not by recency

### 2. Bell icon count matches "Needs Attention" count
As a user, I want the number on the bell icon to match the number shown next to "Needs Attention" so I'm not confused by different counts.

**Acceptance Criteria:**
- Bell badge shows the same `totalCount` as the "Needs Attention" header
- Currently: badge shows `unreadCount` (27) when `hasNew=true`, panel shows `totalCount` (31) — they diverge when some events are acknowledged but not dismissed

### 3. Bell icon respects mine/all filter
As a user, I want the bell icon count to reflect the same mine/all filter as the "Needs Attention" panel, since the panel already switches correctly.

**Acceptance Criteria:**
- Bell badge count comes from the same filtered query used by the panel
- Switching mine/all updates the badge count accordingly
- Currently: the hook `useAttentionEvents(true, filterMine)` already accepts filterMine, but the badge shows `unreadCount` rather than `totalCount`, so the inconsistency makes mine/all appear broken for the badge

### 4. Mine/all switcher affects browser (desktop) notifications
As a user, I want desktop notifications to respect my mine/all preference so I only receive popups for the tasks I care about.

**Acceptance Criteria:**
- Browser notifications only fire for events matching the current filterMine setting
- `newEvents` used in the browser notification `useEffect` comes from the same `useAttentionEvents(true, filterMine)` call — verify this is correct and the `shownRef` deduplication doesn't prevent re-firing when filter changes

### 5. User avatar next to timestamp in "all" mode
As a user, when viewing "all" notifications, I want to see which user is assigned to each task so I can distinguish tasks at a glance.

**Acceptance Criteria:**
- In "all" mode, each notification item shows a small avatar (initials style, matching TaskCard) to the left of the timestamp
- In "mine" mode, the avatar is not shown (single user context)
- Avatar shows assignee initials using the same style as the Kanban card: 20×20px `Avatar` with 0.6rem font, MUI `Avatar` component with initials computed as first+last initial
- If no assignee, fall back to creator's avatar
- Avatar data must come from the event payload (add `assignee_id`, `assignee_name` or similar to the `AttentionEvent` type, or use existing `user_id`/org members data)
