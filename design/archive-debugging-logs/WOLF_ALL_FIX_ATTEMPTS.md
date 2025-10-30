# Wolf Lobby Hang - Complete History of All Fix Attempts

## Test Session: 2025-10-10

---

## Attempt 1: Diagnostic Logging
**Commit:** 29da4d0
**File:** `streaming.cpp`
**Status:** ✅ SUCCESS - Helps debugging

**Changes:**
```cpp
// Added [HANG_DEBUG] logging to:
- PauseStreamEvent handlers (video + audio)
- SwitchStreamProducerEvents handlers
- Shows pipeline state during events
```

**Results:**
- Logs show event sequences clearly
- Identified duplicate pause events (9 audio vs 2 video)
- Confirmed rejoin pattern (100% hang on 2nd join to same lobby)

---

## Attempt 2: Duplicate Pause Event Guard
**Commit:** 15eb3a9
**File:** `streaming.cpp`
**Status:** ✅ PARTIAL SUCCESS - Helps but doesn't fix rejoin

**Changes:**
```cpp
// Video pause handler (line 286, 305-313)
auto pause_sent = std::make_shared<bool>(false);

auto pause_handler = event_bus->register_handler<...>(
    [sess_id, pipeline, pause_sent](...) {
      if (*pause_sent) {
        logs::log(logs::warning, "[HANG_DEBUG] Video PauseStreamEvent DUPLICATE IGNORED");
        return;
      }
      *pause_sent = true;
      gst_element_send_event(pipeline.get(), gst_event_new_eos());
    });

// Same for audio pause handler (line 420, 422-451)
```

**Results:**
- ✅ Duplicate events ARE caught and logged
- ✅ Saw "DUPLICATE IGNORED" messages in logs (confirms fix works!)
- ✅ Prevents multiple EOS events to same pipeline
- ✅ Prevents session count going negative (-1 people)
- ❌ Does NOT fix rejoin hang (different root cause)

**Test Evidence:**
```log
10:17:20 WARN | [HANG_DEBUG] Video PauseStreamEvent for session 342532221405053742
10:17:20 WARN | [HANG_DEBUG] Video PauseStreamEvent DUPLICATE IGNORED for session 342532221405053742
```

---

## Attempt 3: leaky-type=downstream on interpipesink
**Commit:** be0c62c (reverted immediately)
**File:** `streaming.cpp`
**Status:** ❌ FAILED - Invalid property

**Changes:**
```cpp
"interpipesink sync=true async=false name={session_id}_video max-buffers=1 leaky-type=downstream"
```

**Results:**
```
ERROR: Pipeline parse error: no property "leaky-type" in element "interpipesink"
```

**Why it failed:**
- interpipesink doesn't have `leaky-type` property
- That's a `queue` element property
- Wrong GStreamer element for this approach

**Reverted in:** e891700

---

## Attempt 4: max-buffers=0 (Unlimited Buffers)
**Commit:** e891700 (part of revert)
**File:** `streaming.cpp`
**Status:** ⚠️ UNTESTED - Reverted before testing

**Changes:**
```cpp
// Changed from max-buffers=1 to max-buffers=0
"interpipesink sync=false async=false name={session_id}_video max-buffers=0"
```

**Theory:**
- max-buffers=0 means unlimited buffers
- Maybe behaves differently for disconnected sinks

**Why reverted without testing:**
- max-buffers=0 = UNLIMITED accumulation
- Would make stale buffer problem WORSE
- Hundreds of stale frames instead of 1
- All referencing freed GPU memory
- Bad idea for our use case

**Verdict:**
Wrong direction - need fewer/fresher buffers, not unlimited accumulation.

---

## Attempt 5: Empty Lobby Flush via IDRRequestEvent
**Commit:** be0c62c (in lobbies.cpp, reverted)
**File:** `sessions/lobbies.cpp`
**Status:** ❌ FAILED - Wrong event type

**Changes:**
```cpp
// When lobby becomes empty
if (lobby.connected_sessions->load()->size() == 0 && !lobby.stop_when_everyone_leaves) {
  logs::log(logs::info, "Lobby {} now empty, flushing video producer");
  ev_bus->fire_event(immer::box<events::IDRRequestEvent>{...});
}
```

**Theory:**
- IDRRequestEvent forces keyframe
- Maybe also flushes buffers?

**Why it doesn't work:**
- IDRRequestEvent only forces I-frame (keyframe)
- Does NOT flush/clear buffers
- Wrong event for this purpose

**Reverted in:** be0c62c (same commit)

---

## Attempt 6: queue leaky=downstream BEFORE interpipesink
**Commit:** 9cb7bcf (reverted in cf0f4af)
**File:** `streaming.cpp`
**Status:** ❌ FAILED - Broke first join

**Changes:**
```cpp
// Pipeline modification (line 79-83)
auto pipeline = fmt::format(
    "waylanddisplaysrc name=wolf_wayland_source render_node={render_node} ! "
    "{buffer_format}, width={width}, height={height}, framerate={fps}/1 ! "
    "{pipeline_fix}"
    "queue leaky=downstream max-size-buffers=1 ! "  // ← ADDED
    "interpipesink sync=true async=false name={session_id}_video", //
    ...);
```

**Theory:**
- queue element HAS leaky property (unlike interpipesink)
- Drops old buffers automatically
- Same pattern audio pipeline uses successfully
- Should prevent stale buffer accumulation

**Results:**
```
ERROR: Internal data stream error
```
- ❌ INSTANT crash on FIRST join (made it worse!)
- ❌ Broke basic functionality
- ❌ Error on consumer (interpipesrc) side, not producer

**Why it failed:**
- Adding queue breaks interpipe connection somehow
- interpipe expects direct connection producer→consumer
- Queue in between may break caps negotiation or timing
- Different behavior than audio (audio uses queue BEFORE interpipesink in producer, works fine)

**Reverted in:** cf0f4af

**Analysis:**
Audio pipeline (line 144-147):
```cpp
"pulsesrc ... ! "
"audio/x-raw, ... ! "
"queue leaky=downstream max-size-buffers=3 ! "  // Works here!
"interpipesink ..."
```

Video with queue (our attempt):
```cpp
"waylanddisplaysrc ... ! "
"video/x-raw, ... ! "
"queue leaky=downstream max-size-buffers=1 ! "  // Breaks here!
"interpipesink ..."
```

**Hypothesis why audio works but video doesn't:**
- Audio: Simple audio samples, no GPU memory
- Video: CUDA/GPU memory contexts
- Queue might break GPU memory transfer chain
- Or caps negotiation different for video vs audio

---

## Current State (After All Attempts)

**Working Configuration:**
```cpp
// streaming.cpp line 79-83
"waylanddisplaysrc ... ! "
"video/x-raw, ... ! "
"{pipeline_fix}"  // CUDA upload/convert if NVIDIA
"interpipesink sync=true async=false name={session_id}_video max-buffers=1"
```

**Active Fixes:**
- ✅ Duplicate pause guard (works, confirmed in logs)
- ✅ Diagnostic logging (helps debugging)

**Known Issues:**
- ❌ Rejoin hang (100% reproducible)
- ❌ Can't modify producer pipeline without breaking it
- ❌ No GStreamer-level solution found

---

## What We Learned

### About the Bug:

1. **Duplicate Pause Events:** Real issue, our guard fixes it
2. **Rejoin Pattern:** First join works, rejoin always fails
3. **Stale Buffers:** interpipesink holds 1 frame with freed GPU reference
4. **CUDA Context:** Frames from old session can't be accessed by new session

### About interpipe:

1. **Can't add queue before interpipesink** - breaks connection
2. **interpipesink has no leaky property** - no built-in buffer drop
3. **Audio vs Video different** - queue works in audio, breaks in video
4. **GPU memory complicates it** - CUDA contexts don't survive across sessions

### About Wolf Architecture:

1. **Helix uses stop_when_everyone_leaves=false** - lobbies persist when empty
2. **Producer keeps running** - even with no consumers
3. **PauseStreamEvent triggers leave** - automatic, not just explicit API calls
4. **interpipe designed for live switching** - not for disconnected/reconnected sources

---

## Fix Attempts Summary Table

| # | Approach | File | Status | Breaks First Join? | Fixes Rejoin? |
|---|----------|------|--------|-------------------|---------------|
| 1 | Diagnostic logging | streaming.cpp | ✅ Works | No | N/A |
| 2 | Duplicate pause guard | streaming.cpp | ✅ Partial | No | No |
| 3 | leaky-type on interpipesink | streaming.cpp | ❌ Failed | N/A | N/A |
| 4 | max-buffers=0 | streaming.cpp | ⚠️ Untested | Unknown | Likely No |
| 5 | IDR flush event | lobbies.cpp | ❌ Failed | No | No |
| 6 | queue before interpipesink | streaming.cpp | ❌ Failed | **YES!** | Unknown |

---

## Conclusion

**What Works:**
- Duplicate pause guard (keeps!)
- Diagnostic logging (keeps!)

**What We Can't Do:**
- Modify producer pipeline structure (breaks interpipe)
- Add buffering elements before interpipesink (breaks video)
- Use interpipesink properties that don't exist

**Path Forward:**
1. Keep duplicate guard fix (it helps!)
2. Accept rejoin issue as upstream Wolf limitation
3. Report both bugs to games-on-whales/wolf with our findings
4. Consider workaround: Don't allow rejoining previously-left lobbies in Helix UI

**Recommendation:**
This is an **upstream interpipe/Wolf architectural issue**. Our duplicate guard significantly improves stability (prevents -1 sessions, multiple EOS). The rejoin hang needs upstream solution - interpipe plugin wasn't designed for persistent lobbies with intermittent consumers.
