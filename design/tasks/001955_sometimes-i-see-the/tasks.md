# Implementation Tasks

- [~] Add `export GIT_MERGE_AUTOEDIT=no` near the top of `desktop/shared/helix-workspace-setup.sh` (after `set -e`, before any git command runs)
- [ ] Leave both `git pull` call sites (lines 382 and 473) and their failure handling unchanged — they will now auto-merge silently when possible and hard-fail on real merge conflicts, which is the desired behaviour
- [ ] Rebuild the desktop image with `./stack build-ubuntu` so new sessions pick up the script change
- [ ] Manually verify a fresh agent session starts cleanly with no editor prompt
- [ ] Verify the auto-merge case: create a non-conflicting local commit on the base branch, push origin ahead, restart session, confirm a merge commit is created silently and startup proceeds
- [ ] Verify the conflict case: create a conflicting local commit, restart session, confirm startup hard-fails with the existing FATAL message rather than opening vim
