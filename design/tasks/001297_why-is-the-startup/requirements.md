# Requirements: Fix Startup Script Failure

## Problem Statement

The `./stack start` command fails inside Helix-in-Helix environments because `~/.docker/` is created with root ownership by `17-start-dockerd.sh`, then the `retro` user can't access it.

Errors:
1. `Permission denied` when accessing `~/.docker/config.json`
2. `unknown flag: --provenance` - BuildKit plugin fails to load

## Root Cause

`17-start-dockerd.sh` runs as root and creates the `helix-shared` buildx builder, which writes to `~/.docker/`. The `retro` user then cannot read this directory, breaking Docker CLI plugin loading.

## User Stories

### US1: Developer can run stack commands in Helix-in-Helix
As a developer using Helix-in-Helix, I want `./stack start` to work without permission errors.

**Acceptance Criteria:**
- [ ] `~/.docker/` is owned by `retro` user after `17-start-dockerd.sh` completes
- [ ] `docker buildx version` works as `retro` user
- [ ] `./stack start` completes successfully

## Solution

Modify `17-start-dockerd.sh` to `chown` the `.docker` directory to the `retro` user after creating the buildx builder.

## Scope

- **In scope:** Fix `helix/desktop/shared/17-start-dockerd.sh` to set correct ownership
- **Out of scope:** Other permission issues, host-level Docker problems