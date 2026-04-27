# Implementation Tasks

- [x] Add `export GIT_MERGE_AUTOEDIT=no` near the top of `desktop/shared/helix-workspace-setup.sh` (after `set -e`, before any git command runs)
- [x] Leave both `git pull` call sites (lines 382 and 473) and their failure handling unchanged — they will now auto-merge silently when possible and hard-fail on real merge conflicts, which is the desired behaviour
- [x] Verify locally with a TTY-faked repro that `GIT_MERGE_AUTOEDIT=no` suppresses the editor for auto-mergeable pulls **and** still hard-fails on real merge conflicts (see "Implementation Notes" in design.md)
- [ ] (Deployment, left to reviewer) Rebuild the desktop image with `./stack build-ubuntu` so new sessions pick up the script change
- [ ] (Deployment, left to reviewer) Start a fresh agent session against a workspace where the base branch has diverged from origin, and confirm startup completes without opening vim
