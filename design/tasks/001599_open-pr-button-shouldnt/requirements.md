# Requirements: Hide "Open PR" Button Until Agent Has Pushed

## User Story

As a user reviewing an agent's implementation, I should not see the "Open PR" button until the agent has actually pushed at least one commit to its feature branch, so that I don't accidentally trigger a PR with no code changes.

## Problem

The "Open PR" button (and its direct-push variant "Accept") appears immediately when a task enters `implementation` status. At this point, the feature branch may exist locally but the agent hasn't pushed any commits yet. Clicking the button in this state would either create an empty PR or fail confusingly.

## Acceptance Criteria

- [ ] Both the "Open PR" / "Accept" button and the "Reject" button are disabled (with a tooltip) when the task has no `last_push_at` value
- [ ] Both buttons enable as soon as `last_push_at` is set (i.e., the agent has pushed at least one commit)
- [ ] Both inline and full-size button variants respect this condition
- [ ] No backend changes required — `last_push_at` is already tracked and exposed via the API
