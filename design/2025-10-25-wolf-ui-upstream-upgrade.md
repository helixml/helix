# Wolf UI Upstream Upgrade - October 25, 2025

## Pre-Upgrade State

### Repository: ~/pm/wolf

**Previous Working Branch:** `stable-moonlight-web`
- **Last Commit:** `eb78bcc` - Refactor: Extract pipeline defaults into shared compute_pipeline_defaults function
- **Purpose:** Stable branch for "apps" mode with Moonlight web integration
- **Key Features:**
  - Auto-pairing PIN support (commits 7ca42f8, 6594b13)
  - Phase 5 HTTP support for Moonlight pairing (commit 260ec5a)
  - Pipeline defaults respecting WOLF_USE_ZERO_COPY env var (commits befda29, b54ec5b, eb78bcc)

**Target Branch:** `wolf-ui-working`
- **Before Upgrade:** `bc2d8aa` - Add apps field to SystemMemoryResponse for frontend compatibility
- **After Upgrade:** `63ce049` - Merge upstream wolf-ui: Nvidia performance improvements
- **Purpose:** Testing upstream wolf-ui lobbies mode with performance improvements

### Upgrade Summary

**Merged from upstream wolf-ui (games-on-whales/wolf):**
- Commit range: e26a5f5..4c6f40f
- **Major Features:**
  - New zero copy pipeline for Nvidia GPUs (9f726f8)
  - Upgraded GStreamer to 1.26.7 (daef0be)
  - Dynamic CUDA linking with graceful fallback (7da2d3f)
  - CUDA device ID derivation from render node (9d400ed)
  - Upgraded Boost to latest version (1d7ba8f)
  - New gst-video-context module for CUDA context management

**Conflict Resolution:**
- File: `src/moonlight-server/streaming/streaming.cpp`
- Resolution: Kept both our duplicate pause event guard fix AND upstream's new video context handling
- Result: Both features preserved in merged code

### Important Features Already in wolf-ui-working

✅ **System memory debugging endpoint** (82eecca)
- Added for Agent Sandboxes dashboard
- Returns memory stats with apps field for frontend compatibility

✅ **Duplicate PauseStreamEvent fix** (15eb3a9)
- Prevents session corruption from duplicate pause events
- Includes diagnostic logging for hang investigation

✅ **Auto-pairing support** (c19557a, 57321eb)
- MOONLIGHT_INTERNAL_PAIRING_PIN environment variable
- Auto-pairs with moonlight-web-stream without manual PIN entry

✅ **Pipeline defaults** (f852146, b07ccc7)
- compute_pipeline_defaults function
- Respects WOLF_USE_ZERO_COPY configuration

### Features from stable-moonlight-web - REVIEW COMPLETE ✅

All important commits from `stable-moonlight-web` have been reviewed:

1. **eb78bcc** - Refactor: Extract pipeline defaults into shared compute_pipeline_defaults function
   - ✅ ALREADY IN wolf-ui-working as b07ccc7 (same implementation)

2. **b54ec5b** - Fix API endpoint to build complete pipeline defaults based on WOLF_USE_ZERO_COPY
   - ✅ ALREADY IN wolf-ui-working as f852146 (same implementation)

3. **befda29** - Fix API endpoint defaults to respect WOLF_USE_ZERO_COPY env var
   - ✅ BETTER IMPLEMENTATION in wolf-ui-working
   - stable-moonlight-web: Reads env var directly in endpoint_AddApp
   - wolf-ui-working: Uses compute_pipeline_defaults() function (cleaner refactoring)
   - **Conclusion:** wolf-ui-working version is superior, no cherry-pick needed

4. **63874f4** - WIP on a stable branch which also works with moonlight-web
   - ℹ️ SKIP: WIP commit, not production ready

5. **6594b13, 7ca42f8, 260ec5a** - Auto-pairing and Phase 5 HTTP support
   - ✅ ALREADY IN wolf-ui-working (c19557a, 57321eb - identical implementation)

6. **2984c2b** - feat: upgraded gst-wayland-comp
   - ✅ NO CHANGE: Both branches use same gst-wayland-display repository clone
   - Upstream wolf-ui may have newer version built-in, but our custom build is current

**Summary:** No cherry-picks needed. wolf-ui-working has all features from stable-moonlight-web, often with better implementations.

### Testing Plan

**Phase 1: Apps Mode Testing** (Current)
- Test Wolf in "apps" mode with new Nvidia performance improvements
- Verify streaming performance and stability
- Monitor for any regressions from upstream merge

**Phase 2: Lobbies Mode Testing** (Next)
- Once apps mode is stable, test lobbies mode functionality
- This is the ultimate goal for production deployment

### Build Status

✅ Wolf container rebuilt successfully with new code
✅ Image pushed to registry: `registry.helixml.tech/helix/wolf:f3d4e7847`
✅ Container restarted and ready for testing

### Next Steps

1. Test apps mode stability with new wolf-ui upstream code
2. Verify Nvidia performance improvements are working
3. Review befda29 and 2984c2b for any missing changes
4. Once stable in apps mode, proceed to lobbies mode testing
5. Push wolf-ui-working branch changes to remote repository

### References

- Upstream Wolf: https://github.com/games-on-whales/wolf/tree/wolf-ui
- Helix Wolf Fork: https://github.com/helixml/wolf/tree/wolf-ui-working
- Local Path: ~/pm/wolf
