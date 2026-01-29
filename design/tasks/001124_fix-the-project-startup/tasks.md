# Implementation Tasks

## Investigation & Setup

- [ ] Understand exact execution context: where/how the startup script is invoked by the API
- [ ] Verify the helix-specs worktree setup is correct
- [ ] Document the expected directory structure in comments

## Fix Docker Compose Shim

- [ ] Edit `/home/retro/work/helix/desktop/docker-shim/compose.go`
- [ ] Remove the pluginName from finalArgs when calling docker-compose.real
- [ ] Test: `docker compose -f docker-compose.dev.yaml config --services`
- [ ] Commit to main branch (separate PR/issue)
- [ ] Rebuild docker-shim: `cd desktop/docker-shim && go build -o /usr/local/bin/docker-shim`

## Fix Startup Script

- [ ] Edit `/home/retro/work/helix-specs/.helix/startup.sh`
- [ ] Add check to ensure we're finding the correct helix main repo
- [ ] Add check to ensure helix repo is on main branch before building
- [ ] Improve yarn installation handling (check if already installed first)
- [ ] Add better error messages for each failure point
- [ ] Make tmux session creation idempotent (check if already exists)
- [ ] Test the script end-to-end

## Testing

- [ ] Run startup script on fresh clone (simulating new project setup)
- [ ] Run startup script again to verify idempotency
- [ ] Verify docker compose commands work
- [ ] Verify Helix stack builds and starts
- [ ] Test with and without privileged mode

## Cleanup & Documentation

- [ ] Add comments explaining the worktree setup
- [ ] Document any remaining manual steps
- [ ] Commit changes to helix-specs branch
- [ ] Push to origin helix-specs branch
