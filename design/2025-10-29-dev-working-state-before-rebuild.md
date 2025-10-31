# Dev Environment Working State - Before Rebuild

**Date:** 2025-10-29 22:20 GMT
**Status:** Known working state - lobby reconnect works fine in dev
**Purpose:** Snapshot before rebuilding dev with recent changes

## Context

Dev is working correctly with lobby reconnect (disconnect → reconnect works fine).
Prod has format mismatch errors on reconnect. We've made several commits to fix prod,
and want to ensure those changes don't break dev.

## Current Running State (WORKING)

### Wolf Container
- **Image SHA:** `309c34625efb929219c53779365fa780bb73431f96f334a35f53d401406e3200`
- **Tag:** `registry.helixml.tech/helix/wolf:2.5.0-rc7`
- **Built:** 25 hours ago (2025-10-28 ~21:20 GMT)
- **Running:** 2 hours (started 2025-10-29 ~20:20 GMT)
- **Status:** Working - lobby reconnect successful

### Git State

**Helix repo** (`/home/luke/pm/helix`):
- **HEAD:** `615bb8162` - Fix lobby reconnect format mismatch: Add CUDA buffer caps to Wolf UI
- **Branch:** feature/helix-code
- **Recent commits (last 3 hours):**
  - `615bb8162` - Fix lobby reconnect format mismatch: Add CUDA buffer caps to Wolf UI (just committed)
  - `9324f71e2` - Update CLAUDE.md rules and add pairing architecture design doc
  - `964a89912` - Security: Add TURN_PASSWORD to moonlight-web and require credentials

**Wolf repo** (`/home/luke/pm/wolf`):
- **HEAD:** `bab25a4` - Add init-wolf-config.sh to dockerignore whitelist
- **Branch:** wolf-ui-working
- **Recent commits (last 25 hours):**
  - `bab25a4` - Add init-wolf-config.sh to dockerignore whitelist (30 min ago)
  - `57b3d81` - Add Helix init script to Wolf image for config.toml initialization
  - `03745f5` - Enable CUDA buffers for Wolf UI and add resolution change support (25 hours ago)
  - `488347e` - Fix race condition in SwitchStreamProducerEvents
  - `49c18f9` - Fix duplicate SwitchStreamProducerEvents causing GStreamer refcount errors

**Moonlight-web-stream repo** (`/home/luke/pm/moonlight-web-stream`):
- **HEAD:** `a5cf63f` - Security: Require credentials from env vars, no insecure fallbacks
- **Branch:** feature/kickoff
- **Recent commits:**
  - `a5cf63f` - Security: Require credentials from env vars (today)
  - `037e8ee` - Build template files into container image (today)
  - `1019a7b` - Fix /api/pair endpoint 404: Add Config to api_service scope (today)

## Wolf Image Analysis

The running rc7 image (309c34625efb) was built **25 hours ago**, which corresponds to
the time of commit `03745f5` (Enable CUDA buffers and allow-renegotiation).

**Likely Wolf source state for rc7:**
- Includes: `03745f5` and earlier (CUDA buffer code fix, allow-renegotiation in C++ code)
- Does NOT include: `57b3d81`, `bab25a4` (recent dockerignore and init script changes)

**Key difference from latest:**
- rc7 has the C++ code fixes (allow-renegotiation in streaming.cpp)
- rc7 does NOT have the latest dockerignore fix
- But rc7 builds successfully (dockerignore issue was introduced later)

## Config Files (Runtime Generated)

### Dev Wolf Config
**File:** `/home/luke/pm/helix/wolf/config.toml` (not in version control)
**Status:** Generated from template on first run
**Key setting:** Does NOT have `video_producer_buffer_caps = 'video/x-raw(memory:CUDAMemory)'`
**Still works:** Yes - the C++ code fix (allow-renegotiation) compensates

### Dev Wolf Template
**File:** `/home/luke/pm/helix/wolf/config.toml.template` (version controlled)
**Current state:** NOW includes `video_producer_buffer_caps` (commit 615bb8162)
**Previous state:** Did NOT include it when dev config was generated

## Why Dev Works Without video_producer_buffer_caps

The rc7 Wolf binary includes commit `03745f5` which added:

1. **Code fix:** `allow-renegotiation=TRUE` on interpipesrc (handles format changes)
2. **Config fix:** `video_producer_buffer_caps` in default config

Dev's config doesn't have #2, but the code has #1, so it works.

## Comparison to Prod

**Prod environment:**
- **Wolf:** rc16 (built 30 min ago, NEWER than dev)
- **Config:** Missing `video_producer_buffer_caps` (same as dev)
- **Behavior:** FAILS on reconnect with format mismatch

**Why prod fails but dev works:**
- Both missing config setting
- rc16 code should have allow-renegotiation fix
- Possible: rc16 built from different commit? Or config template differences?

## Changes Made Today (Not in Running Dev)

**Helix repo:**
- `615bb8162` - Added `video_producer_buffer_caps` to wolf/config.toml.template
- `9324f71e2` - Updated CLAUDE.md rules
- `964a89912` - Added TURN_PASSWORD to docker-compose files

**Wolf repo:**
- `bab25a4` - Fixed .dockerignore to include init-wolf-config.sh
- `57b3d81` - Added init script to Wolf image

**Moonlight-web-stream repo:**
- `a5cf63f` - Security: require credentials from env vars
- `037e8ee` - Build template files into container image
- `1019a7b` - Fix /api/pair endpoint 404

## Next Steps

1. **Rebuild dev Wolf** from latest wolf-ui-working branch (includes bab25a4)
2. **Test lobby reconnect** - does it still work?
3. **If broken:** Revert to rc7 image (309c34625efb)
4. **If working:** Deploy rc17 to prod with config fix

## Hypothesis

Dev works because:
- Wolf rc7 binary has `allow-renegotiation=TRUE` in C++ code
- This handles format changes dynamically during interpipe switching
- Config setting `video_producer_buffer_caps` helps but isn't strictly required when code fix is present

Prod fails because:
- Wolf rc16 binary may not have the allow-renegotiation fix applied correctly?
- Or config is generated differently in prod?
- Or runtime state differs?

## Commands to Restore This State

If rebuild breaks dev:

```bash
# Stop current Wolf
docker compose -f docker-compose.dev.yaml down wolf

# Restore working rc7 image
docker tag 309c34625efb wolf:helix-fixed

# Start Wolf with restored image
docker compose -f docker-compose.dev.yaml up -d wolf
```

## Files to Watch

**Critical runtime files (not in git):**
- `/home/luke/pm/helix/wolf/config.toml` - Generated from template, contains Wolf UI app config
- `/home/luke/pm/helix/moonlight-web-config/config.json` - Generated by init script
- `/home/luke/pm/helix/moonlight-web-config/data.json` - Moonlight host pairing state

**Version controlled templates:**
- `/home/luke/pm/helix/wolf/config.toml.template` - NOW has buffer caps fix
- `/home/luke/pm/helix/moonlight-web-config/config.json.template` - In moonlight-web-stream repo
- `/home/luke/pm/helix/moonlight-web-config/data.json.template` - In moonlight-web-stream repo
