# Enforce conventional commit format across helix code repos

## Summary

Claude Code agents previously used conventional commits inconsistently — sometimes `feat: add X`, sometimes `Progress update`. This adds three layers of guidance/enforcement so all helix task commits follow conventional format.

## Changes

- **CLAUDE.md**: Added explicit conventional commit rule with type list and examples in the "Commits & Debugging" section.
- **`desktop/shared/helix-git-hooks.sh`**: Extended `install_code_repo_hook` to validate the conventional commit regex before adding the existing `Spec-Ref` trailer. Skips merge/revert/fixup/squash/amend commits. Helpful error message points users at the correct format. Subject length >72 chars triggers a warning (not failure).
- **`api/pkg/services/agent_instruction_service.go`**: Added a "use conventional commit format" rule to the agent prompt's CRITICAL RULES section, and converted 4 example commit messages in the template to conventional format (e.g., `"Progress update"` → `"chore(specs): update progress"`).

## Why three layers?

- **CLAUDE.md** — read by Claude Code on session start; guides behavior immediately.
- **Agent prompts** — sent by the helix backend to agents during planning/implementation.
- **Hook** — final enforcement; rejects non-conforming commits with a helpful error.

PR titles (set via `pull_request.md`, task 001320) intentionally stay as descriptive prose — PRs summarize a body of work, conventional commits describe atomic changes.

## Testing

The hook was tested manually with 7 cases (no-scope, slashed scope, breaking marker, multi-line body, random text, `WIP`, long subject). All behaved as expected. `go build ./api/pkg/services/` passes.

## Follow-up

After merging, contributors with existing local clones will continue running the old hook until startup re-installs it. Re-running `install_helix_git_hooks` (or restarting the dev environment) picks up the new validator.
