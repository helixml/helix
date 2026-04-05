# Requirements: Notification Fixes

## User Stories

### 1. ~~Correct notification sort order~~ (FIXED in main)
Already resolved — the backend query now wraps `DISTINCT ON` in a subquery with outer `ORDER BY created_at DESC`.

### 2. Bell icon count matches "Needs Attention" count
As a user, I want the number on the bell icon to match the number shown next to "Needs Attention" so I'm not confused by different counts.

**Acceptance Criteria:**
- Bell badge shows the same `totalCount` as the "Needs Attention" header
- Currently: badge shows `unreadCount` when `hasNew=true`, panel shows `totalCount` — they diverge when some events are acknowledged but not dismissed (e.g., bell shows 27, panel shows 31)

### 3. ~~Mine/all switcher affects browser notifications~~ (Already working)
Verified: `newEvents` in `useAttentionEvents.ts` is derived from the same `query.data` that uses `filterMine`. Browser notifications already respect the mine/all toggle. No fix needed.

### 4. User avatar next to timestamp in "all" mode
As a user, when viewing "all" notifications, I want to see which user is assigned to each task so I can distinguish tasks at a glance.

**Acceptance Criteria:**
- In "all" mode, each notification item shows a small avatar (initials style, matching TaskCard) to the left of the timestamp
- In "mine" mode, the avatar is not shown (redundant in single-user context)
- Avatar shows assignee initials using same style as Kanban card: MUI `Avatar`, ~16–20px, 0.5–0.6rem font
- If no assignee, show no avatar (don't fall back to creator — only assignee is meaningful for "who's working on it")
- Backend must provide assignee info on the `AttentionEvent` payload (currently absent)
