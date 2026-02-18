# Implementation Tasks

- [x] Add `chown` command to `helix/desktop/shared/17-start-dockerd.sh` after buildx setup (around line 158)
- [x] Check if `retro` user exists before chown
- [x] Check if `/home/retro/.docker` exists before chown
- [ ] Verify `~/.docker` is owned by `retro:retro` after starting new session
- [ ] Verify `./stack start` completes without permission errors