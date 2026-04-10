# Implementation Tasks

- [x] In `SpecTaskDetailContent.tsx`, import `useAccount` and get `orgMembers` from the account context
- [x] Look up the `created_by` user ID in `orgMembers` to find the matching `TypesUser` object
- [x] Replace `Author: {task.created_by}` with the user's `full_name || email`, falling back to the raw ID if not found
- [~] Verify the fix in the browser — task detail should show email/name instead of numeric ID
