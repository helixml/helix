# Requirements: Fix Startup Script for New Environments

## Problem Statement

The `./stack start` command fails when Docker's `~/.docker/` directory has incorrect permissions (owned by root instead of user). This commonly happens when Docker is first run with `sudo`.

Errors:
1. `Permission denied` when accessing `~/.docker/config.json`
2. `unknown flag: --provenance` - BuildKit plugin fails to load

## User Stories

### US1: Developer can start Helix in any new environment
As a developer setting up a new dev environment, I want `./stack start` to work automatically without manual permission fixes.

**Acceptance Criteria:**
- [ ] Stack script detects and fixes `~/.docker/` permission issues before Docker commands
- [ ] Fix runs automatically without user intervention
- [ ] Works on fresh environments where `~/.docker/` may not exist or be owned by root
- [ ] `docker buildx` commands succeed after the fix

## Scope

- **In scope:** Modify `./stack` script to auto-fix Docker config permissions
- **Out of scope:** Other Docker installation issues, network problems

## Constraints

- Fix must work without requiring `sudo` password prompt (use `sudo` only if necessary)
- Must not break existing working environments
- Should be idempotent (safe to run multiple times)