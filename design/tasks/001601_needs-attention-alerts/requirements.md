# Requirements: "Needs Attention" Mine/Everyone Toggle

## User Stories

**As a user**, I want to toggle the "Needs Attention" panel between "Only mine" and "Everyone's" alerts, so I can focus on tasks I own without being distracted by alerts for tasks I didn't create.

**As a team member**, I want "Only mine" to show alerts for tasks where I am the creator (and later the assignee), so the filter is meaningful as team usage grows.

## Acceptance Criteria

1. The "Needs Attention" panel header includes a toggle to switch between **"Mine"** and **"All"** views.
2. **"Mine"** shows only alerts for spec tasks where the current user is the creator (`created_by`).
3. **"All"** shows all alerts for the current user (existing behaviour — no change).
4. The toggle selection persists across sessions (localStorage).
5. Default is **"All"** (preserves existing behaviour on first load).
6. The toggle state is respected by the badge count on the bell icon (counts only visible events).
7. **Future-ready**: when `assignee_id` lands, "Mine" should match tasks where the user is assignee first, falling back to creator when no assignee is set.

## Out of Scope

- Filtering by assignee (will be handled separately once assignee feature ships).
- Changes to dismiss/acknowledge/snooze behaviour — these remain unaffected by the filter.
- Browser push notifications — these still fire for all new events regardless of filter.
