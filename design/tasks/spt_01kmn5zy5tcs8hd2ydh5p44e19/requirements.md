# Requirements: Fix Endpoint Type Switching (user → global)

## Problem

When an admin edits an existing provider endpoint and changes its type from `user` to `global`, the `owner` field is not updated. It remains set to the original user ID instead of being changed to `"system"`. As a result:

- The system query that loads global providers (which filters by `owner = "system"`) misses the endpoint.
- The endpoint is invisible to other users — it behaves as a user-scoped endpoint despite being marked `global`.

## User Stories

**As an admin**, I want to change an existing user-scoped provider endpoint to global, so that all users in the system can use it.

**Acceptance Criteria:**
- When an admin updates an endpoint's `endpoint_type` from `user` to `global`, the `owner` field is set to `"system"` and `owner_type` is set to `"system"`.
- When an admin updates an endpoint's `endpoint_type` from `global` to `user`, the `owner` field is set to the admin's user ID and `owner_type` is set to `"user"`.
- After switching to `global`, the endpoint appears in the provider list for all users (not just the original owner).
- After switching to `global`, the endpoint is included in the system provider refresh query.
- The `endpoint_type` field itself is also persisted when updated (currently it may not be).
