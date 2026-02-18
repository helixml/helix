# Implementation Tasks

- [ ] Add `chown` command to `helix/desktop/shared/17-start-dockerd.sh` after buildx setup (around line 158)
- [ ] Check if `retro` user exists before chown
- [ ] Check if `/home/retro/.docker` exists before chown
- [ ] Rebuild desktop image: `./stack build-ubuntu`
- [ ] Start new Helix-in-Helix session to test
- [ ] Verify `~/.docker` is owned by `retro:retro`
- [ ] Verify `./stack start` completes without permission errors