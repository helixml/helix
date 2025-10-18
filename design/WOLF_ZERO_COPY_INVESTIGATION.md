# Wolf Zero-Copy Pipeline Investigation

## Key Discovery: Apps Mode Was Forcing Zero-Copy

**Problem Found:**
Wolf `stable-moonlight-web` branch had hardcoded zero-copy DMABuf pipelines in API endpoint defaults, which conflicted with `WOLF_USE_ZERO_COPY=FALSE` environment variable.

**Root Cause:**
- Commit `cf7b37d` added `video_producer_buffer_caps` field to API schema
- Default value was hardcoded as `"video/x-raw(memory:DMABuf)"` (zero-copy)
- This IGNORED the `WOLF_USE_ZERO_COPY` env var that TOML apps respected
- Result: API apps used zero-copy, TOML apps used non-zero-copy → different behavior

**The Fix (stable-moonlight-web branch):**
- **befda29**: Make API endpoint read `WOLF_USE_ZERO_COPY` env var
- **b54ec5b**: Build complete pipelines (not empty strings)
- **eb78bcc**: Extract shared `compute_pipeline_defaults()` function

Now both TOML apps and API apps use the SAME logic from config.include.toml.

## Apps Mode vs Lobbies Mode Comparison

### Apps Mode (stable-moonlight-web branch)
- **What Helix sends**: MinimalWolfApp with NO pipeline specifications
- **What Wolf does**: Uses defaults from `compute_pipeline_defaults()`
- **Zero-copy control**: Via `WOLF_USE_ZERO_COPY` env var
- **Result**: NOW WORKING after refactor ✅

### Lobbies Mode (wolf-ui-working branch)
- **What Helix sends**: CreateLobbyRequest with `VideoProducerBufferCaps: "video/x-raw"`
- **What Wolf does**: Uses TOML defaults for pipeline building
- **Zero-copy control**: Helix EXPLICITLY sets `video/x-raw` (non-zero-copy)
- **Result**: Was already setting correct caps, BUT lobbies broken for different reason ❌

## Next Steps

1. **Apps mode is now working** with moonlight-qt (desktop client)
   - Streaming works end-to-end
   - Uses non-zero-copy CUDA pipelines correctly
   - Problem: Multiple containers created for same agent when using both moonlight-qt and moonlight-web

2. **WebRTC switchover needs fixing** (PRIORITY)
   - Keepalive sessions work (moonlight-qt can connect)
   - Browser "Join" mode hangs on "Stream connected"
   - Need to make WebRTC peer attachment work for existing Moonlight streams

3. **Lobbies mode investigation** (DEFERRED)
   - Wolf-ui-working branch ALREADY respects WOLF_USE_ZERO_COPY
   - Helix explicitly sets video/x-raw for lobbies
   - Broken for some OTHER reason unrelated to zero-copy
   - Can investigate later after WebRTC switchovers working
