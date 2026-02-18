# Implementation Tasks

- [ ] Add Docker config accessibility check near top of `helix/stack` (after line ~15, before any Docker commands)
- [ ] If `~/.docker` exists but is not readable, set `DOCKER_CONFIG` to `${XDG_CONFIG_HOME:-$HOME/.config}/docker`
- [ ] Create the fallback directory if it doesn't exist
- [ ] Log a warning when using the fallback so users know what happened
- [ ] Test on environment with root-owned `~/.docker/`
- [ ] Test on working environment (verify no change in behavior)