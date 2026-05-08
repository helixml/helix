# Requirements: Merge Default Branch Before PR

## User Stories

1. As a project owner, I want agents to merge the latest default branch (e.g. `main`) into their feature branch before presenting a PR, so that PRs are always up-to-date and merge cleanly.

2. As a project owner, I want the "approved push" instruction to push all repos (not just the primary one), so that PRs are opened for every repo that has changes.

## Acceptance Criteria

- [ ] The implementation prompt includes a step telling the agent to merge the default branch into its feature branch before finishing (for all repos with changes)
- [ ] The "approved push" template (`agent_implementation_approved_push.tmpl`) includes `git fetch origin <default_branch> && git merge origin/<default_branch>` before pushing, for every repo
- [ ] The push template has access to the default branch name (passed through the template data)
- [ ] All repos (primary + non-primary) are pushed in the approved push instruction
- [ ] Existing behavior (rebase_required template) is unchanged — it remains a fallback for conflict resolution
