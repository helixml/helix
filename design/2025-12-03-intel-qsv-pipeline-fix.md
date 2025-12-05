# Intel QSV Pipeline Fix and Config Template Improvements

**Date:** 2025-12-03
**Status:** Implemented

## Summary

Fixed Intel QSV (Quick Sync Video) streaming pipeline failures and improved Wolf config template iteration workflow for development.

## Problem

Streaming on Intel GPUs was failing with:
```
no property "add-borders" in element "msdkvpp"
Pipeline error: Internal data stream error.
```

Additionally, Moonlight Web was failing with:
```
[Stream]: failed to get app list from host: Api(XmlTextNotFound("AppTitle"))
```

## Root Cause Analysis

### 1. msdkvpp Property Error

Commit `e80797f` changed the Intel QSV zero-copy pipeline from `vapostproc` to `msdkvpp` to fix the Intel R/B color channel swap bug (intel/media-driver#644, #1820). However, it kept the `add-borders=true` property which only exists on `vapostproc`, not `msdkvpp`.

**Before (broken):**
```toml
video_params_zero_copy = '''msdkvpp add-borders=true !
video/x-raw(memory:VAMemory), format=NV12, width={width}, height={height}, pixel-aspect-ratio=1/1'''
```

**After (fixed):**
```toml
# msdkvpp uses force-aspect-ratio=true by default (no add-borders property)
video_params_zero_copy = '''msdkvpp !
video/x-raw(memory:VAMemory), format=NV12, width={width}, height={height}, pixel-aspect-ratio=1/1'''
```

### 2. Empty App Title XML Parsing

The placeholder app (app 0) had an empty title:
```toml
title = ''
```

When Wolf returns this in its XML app list response, `<AppTitle></AppTitle>` has no text content, causing Moonlight Web's XML parser to fail with `XmlTextNotFound("AppTitle")`.

**Fix:** Changed to non-empty title:
```toml
title = 'Blank'
```

This is appropriate since the app uses `videotestsrc pattern=black` (black screen) and `audiotestsrc wave=silence` (silence).

## Changes Made

### 1. Fixed msdkvpp pipeline in config templates

**Files:**
- `wolf/config.toml.template` (helix repo)
- `~/pm/wolf/docker/config.toml.template` (wolf repo)

Removed incompatible `add-borders=true` from msdkvpp element.

### 2. Fixed empty app title

**File:** `wolf/config.toml.template`

Changed placeholder app title from `''` to `'Blank'`.

### 3. Added bind mount for faster iteration

**File:** `docker-compose.dev.yaml`

Added bind mount to all sandbox services:
```yaml
- ./wolf/config.toml.template:/opt/wolf-defaults/config.toml.template:ro
```

This allows editing `wolf/config.toml.template` without rebuilding the sway image. Changes take effect after `./stack stop && ./stack start`.

## Pipeline Configuration Architecture

Understanding how GPU pipeline config flows through the system:

1. **config.toml.template** (source of truth)
   - Located at `wolf/config.toml.template` in helix repo
   - Baked into helix-sway image at `/opt/wolf-defaults/config.toml.template`
   - Now also bind-mounted in dev mode for faster iteration

2. **init-wolf-config.sh** (initialization)
   - Runs on container startup
   - Copies template to `/etc/wolf/cfg/config.toml` if it doesn't exist or is empty
   - Substitutes variables (hostname, UUID, PIN, GOP size)

3. **Wolf's compute_pipeline_defaults()** (runtime)
   - Reads config.toml
   - Detects GPU vendor (NVIDIA/Intel/AMD)
   - Selects appropriate `[gstreamer.video.defaults.{nvcodec|qsv|va}]` section

4. **wolf_executor.go** (API integration)
   - Sends empty strings for pipeline config
   - Lets Wolf auto-detect optimal pipeline

## Config Refresh Workflow

The `stack` script already handles config cleanup:

**On `./stack stop` (line 591):**
```bash
rm -f wolf/config.toml
```

**On `./stack start` (line 454):**
```bash
rm -f "$DIR/wolf/config.toml" "$DIR/moonlight-web-config/data.json"
```

So `./stack stop && ./stack start`:
1. Deletes old config.toml
2. Starts container with bind-mounted template
3. init-wolf-config.sh regenerates config.toml from template
4. Wolf starts with fresh config

## Testing

After changes:
1. `./stack stop && ./stack start`
2. Verify config loaded: `docker compose -f docker-compose.dev.yaml exec sandbox-amd-intel cat /etc/wolf/cfg/config.toml | grep msdkvpp`
3. Test streaming via Moonlight Web

### 4. Fixed AppNotFound error - Wolf app ID lookup

**Problem:** After fixing the empty title, streaming failed with:
```
AppNotFound - requested app_id 0 not in list of 2 apps
```

Wolf assigns hash-based IDs to apps (e.g., 372955153 for "Blank", 985743958 for "Select Agent"), not sequential 0, 1. The frontend was requesting `app_id=0` because the backend endpoint `/api/v1/wolf/ui-app-id` was looking for an app titled "Wolf UI" which no longer exists.

**File:** `api/pkg/server/agent_sandboxes_handlers.go`

**Fix:** Updated `getWolfUIAppID` to search for "Blank" first (new placeholder app), with fallback to "Wolf UI" for backward compatibility:

```go
// Find placeholder app by name - prefer "Blank" (new), fall back to "Wolf UI" (legacy)
var foundAppID string
for _, app := range apps {
    if app.Title == "Blank" {
        // Prefer "Blank" app (new placeholder app for WebRTC clients)
        foundAppID = app.ID
        break
    }
    if app.Title == "Wolf UI" && foundAppID == "" {
        // Fall back to "Wolf UI" for backward compatibility
        foundAppID = app.ID
    }
}
```

## Future Work

- The msdkvpp pipeline may still have issues (stream terminated errors observed)
- If msdkvpp doesn't work reliably, may need to revert to vapostproc (with color swap bug) as a fallback
- Consider adding runtime pipeline selection based on what actually works on the specific GPU
