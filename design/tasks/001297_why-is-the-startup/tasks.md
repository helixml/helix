# Implementation Tasks

- [ ] Add `fix_docker_permissions()` function to `helix/stack` (after `setup_dev_networking()` around line 55)
- [ ] Call `fix_docker_permissions` at the start of `build_zed()` function
- [ ] Call `fix_docker_permissions` in the `start` command handler
- [ ] Test on fresh environment with root-owned `~/.docker/`
- [ ] Test on working environment (verify no unnecessary sudo prompts)