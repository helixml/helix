# Implementation Tasks

- [ ] Add `export GIT_MERGE_AUTOEDIT=no` and `export GIT_EDITOR=true` near the top of `desktop/shared/helix-workspace-setup.sh` (after `set -e`, before any git command)
- [ ] Add a brief comment explaining why these env vars are set
- [ ] Rebuild the desktop image with `./stack build-ubuntu` so the change ships in new sessions
- [ ] Verify in a fresh session that startup completes without launching vim, even when the helix-specs worktree's `git pull` results in a merge commit
- [ ] Open a PR against `helixml/helix`
