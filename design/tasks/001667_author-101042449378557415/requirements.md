# Requirements: Display Email Instead of User ID for Task Author

## Problem

The "Author" field on task detail pages displays a raw numeric user ID (e.g., `101042449378557415004`) instead of the user's email address. This is confusing and unhelpful.

## User Stories

**US-1**: As a user viewing a task, I want to see the author's email address (or name) instead of a numeric ID, so I know who created the task.

## Acceptance Criteria

- **AC-1**: The "Author:" line in `SpecTaskDetailContent.tsx` displays the user's email or full name instead of the raw `created_by` user ID.
- **AC-2**: Fallback chain: `full_name` → `email` → truncated user ID (if user lookup fails).
- **AC-3**: No additional API calls per task load — resolve the author from the already-loaded organization members list.
