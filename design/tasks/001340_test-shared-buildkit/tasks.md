# Implementation Tasks

## Setup

- [ ] Create test script `helix/scripts/test-buildkit-cache.sh`
- [ ] Create test Dockerfile at `helix/scripts/test-buildkit-cache.Dockerfile`

## Test Script Implementation

- [ ] Add pre-flight checks (helix CLI exists, API key set, project set)
- [ ] Start first spectask session and wait for it to become ready
- [ ] Run cold build inside session, capture build time
- [ ] Verify builder routing (`docker buildx ls` shows `helix-shared *`)
- [ ] Stop first session
- [ ] Start second spectask session
- [ ] Run warm build inside session, capture build time
- [ ] Compare times and report speedup percentage
- [ ] Stop second session and clean up

## Verification Steps

- [ ] Test script manually on a running Helix stack
- [ ] Confirm cold build populates cache
- [ ] Confirm warm build shows cache hits in build output
- [ ] Document results in PR description

## Documentation

- [ ] Add usage instructions to script header comment
- [ ] Update CLAUDE.md if any new patterns discovered