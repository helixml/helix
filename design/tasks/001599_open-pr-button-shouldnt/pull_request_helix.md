# Disable Open PR and Reject buttons until agent has pushed code

## Summary
The "Open PR" / "Accept" and "Reject" buttons in the implementation phase now stay disabled until the agent has pushed at least one commit to the feature branch. A tooltip ("Waiting for agent to push code...") explains the wait. This prevents users from triggering empty PRs or rejecting work that hasn't started yet.

## Changes
- Added `last_push_at` to `SpecTaskForActions` interface and derived `hasPushed` boolean
- Disabled both Reject and Open PR/Accept buttons when `!hasPushed` (inline + stacked variants)
- Updated all 3 call sites (TaskCard, SpecTaskDetailContent x2) to pass `last_push_at`
