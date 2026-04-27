# Implementation Tasks

- [ ] Export `GIT_MERGE_AUTOEDIT=no` near the top of `desktop/shared/helix-workspace-setup.sh` (right after `set -e` / trap setup, before any git command runs)
- [ ] Change line 382 to `git pull --ff-only origin "$BASE_BRANCH"` and replace the `exit 1` failure path with a warning that logs the divergence and continues
- [ ] Change line 473 to `git -C "$WORKTREE_PATH" pull --ff-only origin helix-specs` (existing failure handling already logs and continues — leave it)
- [ ] Rebuild the desktop image with `./stack build-ubuntu` so new sessions pick up the script change
- [ ] Manually verify a fresh agent session starts cleanly with no editor prompt
- [ ] Verify the divergence case: create a local commit on the base branch, push origin ahead, restart session, confirm warning is logged and startup proceeds without opening vim
