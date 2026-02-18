# Requirements: Fix Startup Script for New Environments

## Problem Statement

The `./stack start` command fails when Docker's `~/.docker/` directory has incorrect permissions (owned by root instead of user). This commonly happens when Docker is first run with `sudo`.

Errors:
1. `Permission denied` when accessing `~/.docker/config.json`
2. `unknown flag: --provenance` - BuildKit plugin fails to load

## Root Cause

When Docker can't read `~/.docker/config.json`, it fails to load CLI plugins including `buildx`. The `--provenance=false` flag requires buildx.

## User Stories

### US1: Developer can start Helix in any new environment
As a developer setting up a new dev environment, I want `./stack start` to work automatically without manual permission fixes.

**Acceptance Criteria:**
- [ ] Stack script works even when `~/.docker/` has wrong permissions
- [ ] No `sudo` prompts required
- [ ] Works on fresh environments
- [ ] `docker build` commands with BuildKit features succeed

## Solution

Set `DOCKER_CONFIG` environment variable to bypass broken config directory when permissions are wrong. This is simpler than fixing permissions and doesn't require `sudo`.

## Scope

- **In scope:** Modify `./stack` script to detect and work around Docker config permission issues
- **Out of scope:** Fixing the underlying permission problem (user can do that manually if they want)