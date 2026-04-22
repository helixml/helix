# Fix commit ownership to use spec approver's identity

## Summary
When User 1 creates a task and User 2 approves specs, commits made during implementation were incorrectly attributed to User 1 (the creator) instead of User 2 (the approver). PRs were correctly created as User 2. This fix ensures both commits and pushes use the approver's identity.

See: https://github.com/helixml/helix/pull/2250

## Changes
- Fall back to `SpecApprovedBy` for push credentials when `ImplementationApprovedBy` is not yet set
- Update git identity (`user.name`/`user.email`) in the running container when specs are approved, using the approver's profile
- Validate GitHub OAuth at spec approval time (same pattern as implementation approval)
- Add `git` to the desktop exec command whitelist
- Add internal `execCommandInDesktop` helper for service-layer container command execution

## Note
Requires `build-ubuntu` to deploy the exec whitelist change. Until then, git identity update will gracefully fall back to the creator's identity.
