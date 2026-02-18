# Requirements: Fix Startup Script Failure

## Problem Statement

The `./stack start` command fails during the Zed build phase with two errors:
1. `Permission denied` when accessing `/home/retro/.docker/config.json`
2. `unknown flag: --provenance` - BuildKit is not loading due to config access issue

## Root Cause Analysis

- `/home/retro/.docker/` directory is owned by `root` instead of `retro` user
- This prevents Docker from loading the config file and CLI plugins (buildx)
- Without buildx, the `--provenance=false` flag is unrecognized

## User Stories

### US1: Developer can start the Helix stack
As a developer, I want `./stack start` to complete successfully so I can run the development environment.

**Acceptance Criteria:**
- [ ] `./stack start` runs without permission errors
- [ ] Zed binary builds via Docker with BuildKit features
- [ ] All containers start correctly

## Fix Required

Fix the Docker config directory permissions:
```bash
sudo chown -R retro:retro /home/retro/.docker
```

This is a one-time fix for this specific environment, not a code change.