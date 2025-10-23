# Wolf Hang Bug - Deep Analysis

## Current Binary State
- MD5: `f9236db634b2724651f6132c8d3cb29a`
- Build time: Just completed with `leaky-type=downstream` fix
- Commits: 3 fixes applied (diagnostic logging, duplicate guard, leaky buffers)

## The Core Problem - What We ACTUALLY Know

### Evidence 1: Multiple Audio Pause Events
```
09:32:41 Audio PauseStreamEvent (1)
09:32:41 Audio PauseStreamEvent (2)
09:32:41 Audio PauseStreamEvent (3)
09:32:41 Audio PauseStreamEvent (4)
09:32:41 Audio PauseStreamEvent (5)
```

**BUT** we saw NO "DUPLICATE IGNORED" messages after adding the guard!

**This means one of two things:**
1. The guard code isn't running (binary not updated) - **BUT MD5 changed!**
2. These are NOT duplicates to the same handler - they're DIFFERENT handlers!

### Evidence 2: Rejoin Pattern (100% Reproducible)
- Join lobby A first time → ✅ Works
- Leave lobby A → ✅ Works
- Join lobby B, C → ✅ Works
- **Rejoin lobby A** → ❌ **HANGS** (100%)

Error: "Internal data stream error" + CUDA buffer copy fails

### Evidence 3: Session Count Goes to -1
- Indicates session counter decremented multiple times
- Likely 5 times (matching 5 audio pause events)
- Counter should only decrement once per leave

## Theory Matrix

### Theory A: Duplicate Events to Same Handler
**Likelihood**: LOW (would show "DUPLICATE IGNORED")

**How it would work:**
- Event bus fires PauseStreamEvent once
- Same handler called 5 times
- My guard flag shared across all calls
- Should see "DUPLICATE IGNORED" in logs

**Test**: We DON'T see this message → Theory probably wrong

### Theory B: Multiple Handlers for Same Session
**Likelihood**: HIGH ⭐

**How it would work:**
- `start_streaming_audio()` called 3-5 times for same session
- Each call registers NEW pause handler with OWN pause_sent flag
- PauseStreamEvent fires once
- ALL 5 handlers receive it
- Each handler has `pause_sent=false` → all send EOS
- My guard doesn't help (each handler has separate flag)

**Why this could happen:**
```cpp
// In lobbies.cpp line 108-115 - runs in DETACHED thread
std::thread([lobby, audio_server, ev_bus, channel_count]() {
  streaming::start_audio_producer(lobby->id, ...);
}).detach();
```

If this detached thread is called multiple times, we get multiple handlers!

**Test needed**: Check if audio producer thread spawned multiple times

### Theory C: Stale Buffer Corruption (Rejoin Issue)
**Likelihood**: HIGH for rejoin pattern ⭐

**Evidence:**
- First join: Always works
- Rejoin: Always fails
- Error: "Failed to map input buffer" (CUDA)
- Buffer size mismatch or stale GPU reference

**Why `stop_when_everyone_leaves=false` matters:**
```cpp
// lobbies.cpp line 60-64
if (lobby.stop_when_everyone_leaves && connected_sessions == 0) {
  // Stop lobby
}
// For Helix: stop_when_everyone_leaves = FALSE
// So lobby producer keeps running with NO consumers!
```

**Interpipesink behavior without consumers:**
```gstreamer
interpipesink max-buffers=1  // Only 1 buffer allowed
// NO leaky-type → Blocks when buffer full
// When no listeners → buffer stays full with STALE frame
```

**On rejoin:**
1. interpipesrc reconnects to interpipesink
2. Tries to pull that 1 buffered frame
3. Frame has GPU memory reference from OLD session's CUDA context
4. New session's CUDA can't access old context's memory
5. "Failed to map input buffer" → crash

## My Fixes - Confidence Assessment

### Fix 1: Duplicate Pause Guard
**Confidence**: 20%
**Status**: Applied but no "DUPLICATE IGNORED" messages

**Why low confidence:**
- Should see log messages if working
- Either binary not updated OR wrong theory
- MD5 changed so binary IS updated
- Therefore Theory B (multiple handlers) more likely

### Fix 2: Rejoin/Empty Lobby Code (removed)
**Confidence**: N/A
**Status**: Removed before building

**Why removed:**
- IDRRequestEvent doesn't flush buffers
- Was wrong approach

### Fix 3: leaky-type=downstream
**Confidence**: 85% ⭐

**Why high confidence:**
- Directly addresses stale buffer problem
- `leaky-type=downstream` drops old buffers when queue full
- Queue WILL be full when no consumers (max-buffers=1)
- Ensures rejoin gets fresh frame, not stale GPU reference

**GStreamer interpipesink documentation:**
```
leaky-type:
  - none: Block when buffer full (DEFAULT - our problem!)
  - upstream: Drop incoming buffers
  - downstream: Drop old buffers from queue ← What we need!
```

**How it fixes rejoin:**
- Lobby has no consumers → buffer queue full
- New frame arrives → leaky-type=downstream drops OLD buffer
- Old stale buffer with invalid GPU reference is discarded
- Only fresh frames with valid GPU context remain
- Rejoin pulls fresh frame → no CUDA error!

## What We Still Don't Understand

### Mystery 1: Why 9 Audio vs 2 Video Pause Events?
- Video has same handler pattern as audio
- Why does audio get 4.5x more events?
- Are there multiple audio pipelines?

**Hypothesis:**
Check if audio producer created multiple times but video only twice.

### Mystery 2: Why Doesn't Duplicate Guard Show Messages?
- Code clearly has the guard
- Binary MD5 changed (new build)
- But no log output

**Possible reasons:**
1. Log level filtering (WARN messages filtered?)
2. Different handlers (not duplicates)
3. Binary optimization removed dead code
4. fmt::format strings not in binary as literals

### Mystery 3: Session Counter Underflow
- What decrements `connected_sessions`?
- Is it in `leave_lobby()` function?
- Does pause event trigger leave?

**Check:**
```cpp
// lobbies.cpp line 273-276
handlers.push_back(app_state->event_bus->register_handler<immer::box<events::PauseStreamEvent>>(
  [=](const immer::box<events::PauseStreamEvent> &pause_stream_event) {
    on_moonlight_session_over(pause_stream_event->session_id);  // ← Calls leave!
  }));
```

**YES!** PauseStreamEvent → triggers leave → decrements counter
If 5 pause events → 5 leaves → counter goes -4!

## The REAL Root Cause (High Confidence)

### Primary Bug: Multiple Audio Producer Handlers
```cpp
// If audio producer thread created 5 times for same lobby:
Thread 1: registers handler1
Thread 2: registers handler2
Thread 3: registers handler3
Thread 4: registers handler4
Thread 5: registers handler5

// PauseStreamEvent fires ONCE
// ALL 5 handlers receive it
// ALL 5 call: on_moonlight_session_over() → leave_lobby()
// Session counter: 1 - 5 = -4 people!
```

### Secondary Bug: Stale Buffers on Rejoin
- Empty lobby keeps producer running
- interpipesink buffers 1 frame with no leaky behavior
- Frame references freed CUDA memory
- Rejoin tries to use stale frame → CUDA error

## Confidence in Current Fix

**leaky-type=downstream: 85% confidence** ✅

**Why it should work:**
1. Drops old buffers automatically
2. Standard GStreamer solution for this exact problem
3. Used in other Wolf pipelines (audio already has it!)
4. Prevents stale GPU references

**Why it might not work:**
1. If buffer not the issue (but all evidence points to it)
2. If CUDA context isolation is the problem (less likely)
3. If pipeline structure itself is wrong

## Alternative/Additional Fixes Needed

### Fix A: Prevent Multiple Producer Threads (Upstream Bug)
**Confidence**: 90% this is needed eventually

**Where:** lobbies.cpp line 108-115
```cpp
// Add guard to prevent multiple audio producer startups
static std::set<std::string> audio_producers_started;
if (audio_producers_started.contains(lobby->id)) {
  logs::log(logs::warning, "Audio producer already started for lobby {}", lobby->id);
  return;
}
audio_producers_started.insert(lobby->id);

std::thread([lobby, audio_server, ev_bus, channel_count]() {
  streaming::start_audio_producer(...);
}).detach();
```

### Fix B: Don't Trigger Leave on PauseStreamEvent for Lobbies
**Confidence**: 70%

**Where:** lobbies.cpp line 273-276
```cpp
handlers.push_back(app_state->event_bus->register_handler<immer::box<events::PauseStreamEvent>>(
  [=](const immer::box<events::PauseStreamEvent> &pause_stream_event) {
    // Check if this session is in a lobby
    immer::vector<events::Lobby> lobbies = app_state->lobbies->load();
    bool in_lobby = false;
    for (const auto& lobby : lobbies) {
      auto sessions = lobby.connected_sessions->load();
      if (sessions contains pause_stream_event->session_id) {
        in_lobby = true;
        break;
      }
    }

    if (!in_lobby) {
      on_moonlight_session_over(pause_stream_event->session_id);
    } else {
      logs::log(logs::debug, "Ignoring pause for lobby session {}", pause_stream_event->session_id);
    }
  }));
```

Lobbies shouldn't auto-leave on pause - they should only leave via explicit LeaveLobbyEvent!

### Fix C: Unregister Handlers on Pipeline Cleanup
**Confidence**: 95% needed for production

Event handlers should be explicitly unregistered when pipelines destroyed.

## Test Plan

1. **Test leaky-type fix NOW** (high confidence)
   - Should fix rejoin hang
   - Try: lobby 1 → 2 → 3 → 1 pattern
   - Look for: CUDA errors gone

2. **If still fails, try Fix B** (don't leave on pause)
   - Lobbies only respond to explicit leave events
   - Prevents accidental counter decrements

3. **Long-term: Report upstream with Fixes A+B+C**
   - Multiple handler registration is upstream bug
   - Our fixes are workarounds

## Why I'm 85% Confident in leaky-type Fix

**Pro:**
- ✅ Directly addresses stale buffer issue
- ✅ Standard GStreamer pattern
- ✅ Audio already uses `leaky=downstream` (queue element)
- ✅ Explains CUDA "Failed to map" perfectly
- ✅ Explains 100% reproducible rejoin hang

**Con:**
- ❓ Doesn't explain multiple pause events (separate issue?)
- ❓ Doesn't prevent multiple handlers (upstream bug)
- ❓ May mask deeper problem

**Verdict**: Will fix the rejoin hang, but multiple handlers issue may remain.
