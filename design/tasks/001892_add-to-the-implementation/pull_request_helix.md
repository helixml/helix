# Merge default branch before presenting PR

## Summary
Agents now merge the latest default branch (e.g. `main`) into their feature branch before pushing code and opening a PR. This prevents stale PRs that can't merge cleanly.

## Changes
- Added merge step to the implementation prompt's "Steps" section (`agent_instruction_service.go`)
- Updated `agent_implementation_approved_push.tmpl` to `git fetch` + `git merge` the default branch before pushing each repo
- Added `baseBranch` parameter to `ImplementationApprovedPushInstruction()` and threaded it through from the call site
- Updated test to verify merge instructions appear in the rendered prompt
