# Requirements

## Problem

When an agent session starts, the workspace setup script can drop the user into a `vim` editor showing a default git merge commit message. The user must press `:wq` to continue, blocking session startup on git knowledge.

## Root cause

`desktop/shared/helix-workspace-setup.sh` runs `git pull` in two places:
- Line 382: `git pull origin "$BASE_BRANCH"` (primary repo, "new branch" mode)
- Line 473: `git pull origin helix-specs` (design-docs worktree pull)

When the local branch and the remote have diverged (each has commits the other doesn't), `git pull`'s default merge strategy creates a merge commit and opens `$EDITOR` (vim) to confirm the auto-generated commit message. There is no terminal interaction expected here, so the script appears to hang. This is especially likely on the helix-specs worktree, where multiple agents push to the same branch concurrently.

## User stories

**As an agent operator**, when I start a session, I want the startup process to never block waiting for me to confirm a git merge commit message in vim, so I don't have to remember `:wq` or interrupt my workflow.

## Acceptance criteria

- A divergent `git pull` (one that requires a merge commit) during session startup completes non-interactively, accepting the default auto-generated merge commit message.
- No `vim`/`vi`/`nano` editor is launched during `helix-workspace-setup.sh` or `.helix/startup.sh` execution under any normal startup scenario.
- Existing behavior is preserved when `git pull` is a fast-forward (no merge needed) — no extra commits are created.
- If a git operation truly fails (e.g., merge conflict), the failure surfaces in the log instead of silently hanging.

## Out of scope

- Changing the pull strategy to rebase (would change the project's documented merge-commit policy in CLAUDE.md).
- Modifying user-level `~/.gitconfig`.
- Touching `git commit` calls — they already use `-m`.
