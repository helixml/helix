# Implementation Tasks

- [~] Add `chown` command to `helix/desktop/shared/17-start-dockerd.sh` after buildx setup (around line 158)
- [~] Check if `retro` user exists before chown
- [~] Check if `/home/retro/.docker` exists before chown
- [ ] Verify `~/.docker` is owned by `retro:retro` after starting new session
- [ ] Verify `./stack start` completes without permission errors