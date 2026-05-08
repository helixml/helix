# Suppress git merge editor prompt during workspace setup

## Summary

Agent session startup intermittently opened vim, asking the user to confirm a git merge commit message and blocking the session until they typed `:wq`. This adds `export GIT_MERGE_AUTOEDIT=no` near the top of `desktop/shared/helix-workspace-setup.sh` so the script's `git pull` calls auto-accept the default merge commit message instead of launching `$EDITOR`.

The bug only surfaced when the local base branch (or `helix-specs` worktree) had diverged from origin in a way git could auto-merge — fast-forward pulls were silent, and conflicts were rare. Because the script runs in a kitty/gnome-terminal (TTY), git's default behaviour is to invoke `$EDITOR` for the merge commit message. `GIT_MERGE_AUTOEDIT=no` is git's documented knob for "use the default message, don't open the editor".

## Behaviour after this change

- **Fast-forward pull** — silent, unchanged.
- **Auto-mergeable divergence** — silent merge commit using git's default message, no editor. Startup proceeds.
- **Real merge conflict** — git can't auto-resolve, exits non-zero, and the existing `|| { echo "FATAL: git pull failed on $BASE_BRANCH"; exit 1; }` at line 382 hard-fails the script. This is the desired permanent-error behaviour — the operator must investigate.

## Changes

- `desktop/shared/helix-workspace-setup.sh` — one `export GIT_MERGE_AUTOEDIT=no` near the top with a comment explaining why.

## Scope

The env var is exported into the script's own process environment only — interactive shells and the agent's later git activity (separate process trees) are unaffected.

## Test plan

- Verified locally with a throwaway git repo using `script(1)` to fake a TTY:
  - Auto-mergeable divergence + `GIT_MERGE_AUTOEDIT=no` → silent merge commit, no editor (sentinel `GIT_EDITOR=false` did not trip).
  - Conflicting divergence + `GIT_MERGE_AUTOEDIT=no` → "Automatic merge failed; fix conflicts" → git exits non-zero (script's existing FATAL handler catches it).
- After merge: `./stack build-ubuntu` to rebake the desktop image, then start a fresh agent session against a workspace whose base branch has diverged from origin and confirm startup completes without opening vim.
