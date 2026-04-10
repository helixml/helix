# Show author email/name instead of numeric user ID in task detail

## Summary
The "Author" field on task detail pages displayed a raw numeric user ID (e.g., `101042449378557415004`). Now it resolves the ID against organization members to show the user's full name or email.

## Changes
- `SpecTaskDetailContent.tsx`: Resolve `task.created_by` user ID via org memberships to display `full_name || email`, falling back to the raw ID if the user isn't found
