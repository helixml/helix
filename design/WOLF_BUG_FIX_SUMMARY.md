# Wolf Lobby Switching Bug - Root Cause and Fix

## Problem

When switching lobbies in Wolf-UI, video randomly freezes with:
- Frozen video (last frame stuck)
- Mouse input stops working
- Lobby shows `-1 people` (negative session count)
- ~10% failure rate with native Moonlight clients
- Higher resolution clients (3024×1890 @ 120fps) trigger it more frequently

## Root Cause Identified

**Multiple Audio PauseStreamEvent calls for the same event**

Evidence from diagnostic logs:
```
09:32:41 Audio PauseStreamEvent (1)
09:32:41 Audio PauseStreamEvent (2)
09:32:41 Audio PauseStreamEvent (3)
09:32:41 Audio PauseStreamEvent (4)
09:32:41 Audio PauseStreamEvent (5)
```

**Why This Causes the Bug:**

1. Each `PauseStreamEvent` calls `gst_element_send_event(pipeline.get(), gst_event_new_eos())`
2. Each EOS event:
   - Shuts down the GStreamer pipeline
   - Decrements the connected session counter
   - Triggers cleanup logic

3. Multiple EOS events on same pipeline:
   - Session counter goes negative (`-1 people`)
   - Pipeline gets corrupted
   - GStreamer state becomes inconsistent
   - Results in "Internal data stream error"

**Error Sequence When Bug Triggers:**
```
ERROR: Failed to map input buffer (CUDA)
ERROR: Failed to copy SYSTEM -> CUDA
ERROR: Internal data stream error
ERROR: streaming stopped, reason error (-5)
```

## Why Audio Events Fire Multiple Times

**Unknown upstream issue** - could be:
- Event bus firing same event multiple times
- Multiple handlers registered for same session
- Handler cleanup not working properly
- Race condition in event dispatch

Regardless of root cause, **handlers must be idempotent** to prevent corruption.

## The Fix

**Added duplicate event guard to both video and audio pause handlers:**

```cpp
// In streaming.cpp - Video pause handler
auto pause_sent = std::make_shared<bool>(false);

auto pause_handler = event_bus->register_handler<immer::box<events::PauseStreamEvent>>(
    [sess_id = video_session->session_id, pipeline, pause_sent](...) {
      if (ev->session_id == sess_id) {
        // Guard against duplicate pause events
        if (*pause_sent) {
          logs::log(logs::warning, "Video PauseStreamEvent DUPLICATE IGNORED");
          return;
        }
        *pause_sent = true;

        // Send EOS only once
        gst_element_send_event(pipeline.get(), gst_event_new_eos());
      }
    });

// Same pattern for Audio pause handler
```

**How This Fixes the Bug:**

- First `PauseStreamEvent` → processes normally, sends EOS
- Subsequent duplicates → logged and ignored
- Session counter only decrements once
- Pipeline receives single EOS as intended
- No corruption, no negative counts

## Testing Evidence

**Test Setup:**
- Wolf-UI session (1920×1080 @ 60fps) via moonlight-web
- Native Moonlight client (3024×1890 @ 120fps) from Mac
- 3 external agent lobbies
- Rapid switching between lobbies

**Before Fix:**
- Audio pause events: **9 duplicate calls**
- Video pause events: **2 calls**
- Result: Frozen video, -1 session count

**After Fix (Expected):**
- Audio pause events: **1 processed, 8 ignored**
- Video pause events: **1 processed, 1 ignored**
- Result: Clean lobby switching, correct session count

## Commit

```
commit 15eb3a9
Author: Luke Marsden <...>

Fix duplicate PauseStreamEvent causing session corruption

ROOT CAUSE: Audio PauseStreamEvent fires 4-5 times when leaving lobby
FIX: Add duplicate event guard using shared bool flag
```

## Test Results: Duplicate Guard Working!

**CONFIRMED - Duplicate Guard Fix IS Working:**
```
10:17:20 WARN | [HANG_DEBUG] Video PauseStreamEvent for session 342532221405053742
10:17:20 WARN | [HANG_DEBUG] Video PauseStreamEvent DUPLICATE IGNORED for session 342532221405053742
```

**Good News:**
- ✅ Duplicate pause events ARE being caught and ignored!
- ✅ Fix is active and logging correctly
- ✅ Prevents multiple EOS events to same pipeline

**Bad News:**
- ❌ Still hangs on REJOIN pattern (different root cause)
- ❌ 100% reproducible when rejoining previously-left lobby
- ❌ Duplicate guard helps but doesn't fully solve it

**Critical Pattern Discovered:**

Sequence that ALWAYS causes hang:
```
1. Join lobby A (first time) → ✅ Works
2. Leave lobby A → ✅ Clean exit
3. Join lobby B → ✅ Works
4. Leave lobby B → ✅ Clean exit
5. Join lobby C → ✅ Works
6. Leave lobby C → ✅ Clean exit
7. Join lobby A (REJOIN) → ❌ HANGS!
```

**Why Rejoin Fails:**

When you leave a lobby:
- Consumer pipeline (client) shuts down via EOS
- **Producer pipeline (lobby) keeps running** (correct - for multi-user)
- Producer's `interpipesink` has **no listeners** but keeps buffering
- **Buffers accumulate** in interpipesink queue

When you rejoin the same lobby:
- `interpipesrc` reconnects to listen to that lobby's `interpipesink`
- **Tries to consume buffered frames** from previous session
- **Frames reference freed GPU memory** (from previous CUDA context)
- CUDA buffer copy fails: "invalid buffer size" / "Failed to map input buffer"
- Pipeline error → video freeze

**Root Cause:**
Lobby producer pipelines need to **flush interpipesink buffers** when connected_sessions reaches 0.

## Attempted Fix #2: leaky-type=downstream (FAILED)

**Attempt:**
Added `leaky-type=downstream` to interpipesink to drop old buffers

**Result:**
```
ERROR: no property "leaky-type" in element "interpipesink"
Pipeline parse error
```

**Why it failed:**
- interpipesink doesn't have a `leaky-type` property
- That property exists on `queue` elements, not interpipesink
- Wrong GStreamer element for this approach

**Reverted** to original `max-buffers=1` configuration

## Next Steps - Remaining Solutions

### Option A: Report to Upstream (Recommended)
Since we have:
- ✅ Duplicate guard working
- ✅ Clear reproduction pattern (rejoin hangs)
- ✅ Diagnostic logs showing the issue

**Report both bugs to games-on-whales/wolf:**
1. Duplicate PauseStreamEvent issue (we have a fix)
2. Rejoin hang from stale buffers (needs upstream solution)

### Option B: Don't Auto-Leave Lobbies on Pause
Modify lobbies.cpp to not trigger leave_lobby() on PauseStreamEvent for lobby sessions.
This may prevent the stale buffer accumulation.

### Option C: Accept 90% Success Rate
- First join: Always works
- Don't rejoin previously-left lobbies
- Workaround: Create new lobbies instead of rejoining

## Files Modified

- `/home/luke/pm/wolf/src/moonlight-server/streaming/streaming.cpp`
  - Added `pause_sent` flag to video pause handler (line 286)
  - Added duplicate guard to video handler (lines 308-313)
  - Added `pause_sent` flag to audio pause handler (line 420)
  - Added duplicate guard to audio handler (lines 425-430)

## Diagnostic Tools Created

- `monitor-wolf-hang.sh` - Real-time hang detection
- `simple-lobby-switch-test.sh` - Automated switching test
- `WOLF_CRASH_INVESTIGATION.md` - Investigation methodology

## Upstream Reporting

If fix successful, report to games-on-whales/wolf:
- **Title**: "Fix duplicate PauseStreamEvent causing session corruption and frozen video"
- **Impact**: Prevents random hangs when switching lobbies
- **Solution**: Add idempotency guard to pause event handlers
- **Evidence**: Logs showing 9 duplicate audio pause events
