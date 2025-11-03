# Wolf Hang Debug - Next Steps

## What I've Done

1. ✅ Created monitoring script: `monitor-wolf-hang.sh`
   - Watches for GStreamer refcount error spikes
   - Auto-captures logs when hang detected
   - Auto-recovers by restarting Wolf
   - Run in background while testing

2. ✅ Added diagnostic logging to Wolf streaming.cpp:
   - Logs pipeline state during PauseStreamEvent
   - Logs every step of SwitchStreamProducerEvents
   - Uses `[HANG_DEBUG]` prefix for easy filtering
   - Shows exact sequence when bug triggers

3. ✅ Updated CLAUDE.md:
   - Clarified Wolf is upstream wolf-ui with minimal changes
   - Only Luke's commits: auto-pairing PIN + Phase 5 HTTP
   - GStreamer bugs are upstream, not from our changes

4. 🔄 Rebuilding Wolf with diagnostic logging (in progress)

## Testing Procedure

### Step 1: Start Monitoring (Terminal 1)
```bash
cd /home/luke/pm/helix
./monitor-wolf-hang.sh
```

Leave this running - it will detect and log hangs automatically.

### Step 2: Manual Testing
From your Mac or iPad:

1. Open `http://node01.lukemarsden.net:8081`
2. Click "Wolf" host
3. Launch "Wolf UI" app
4. Inside Wolf-UI, join different external agent lobbies
5. Repeat joining/switching 10+ times

The monitor will alert when hang occurs and save diagnostic logs to `/tmp/wolf-hang-*.log`

### Step 3: Analyze Results

After reproducing the hang, check the diagnostic logs:
```bash
# Find captured logs
ls -lt /tmp/wolf-hang-*.log | head -3

# View the sequence before hang
cat /tmp/wolf-hang-<timestamp>.log | grep HANG_DEBUG

# Look for the pattern:
# - Was SwitchStreamProducerEvents fired?
# - Did PauseStreamEvent fire during switch?
# - What was pipeline state when it happened?
```

## What the Diagnostic Logging Shows

When lobby switching works correctly, you'll see:
```
[HANG_DEBUG] Video SwitchStreamProducerEvents: session X switching to Y, pipeline state: PLAYING
[HANG_DEBUG] Switching interpipesrc listen-to: interpipesrc_X_video → Y_video
[HANG_DEBUG] Unrefing interpipesrc element
[HANG_DEBUG] Switch complete for session X
```

When the bug triggers, we might see:
```
[HANG_DEBUG] Video PauseStreamEvent for session X, pipeline state: PLAYING → NULL
[HANG_DEBUG] Video SwitchStreamProducerEvents: session X switching to Y, pipeline state: NULL
<-- This is the problem! Switching while pipeline is shutting down
```

Or:
```
[HANG_DEBUG] Switching interpipesrc listen-to: interpipesrc_X_video → Y_video
[HANG_DEBUG] Unrefing interpipesrc element
<-- No "Switch complete" message = crashed during unref
```

## Expected Root Causes

Based on code review, most likely issues:

### 1. Race: PauseStreamEvent During Switch
If PauseStreamEvent fires while SwitchStreamProducerEvents is in progress:
- Switch handler calls `gst_bin_get_by_name()`
- Pause handler sends EOS, pipeline starts teardown
- Switch handler calls `gst_object_unref(src)`
- But `src` is already being destroyed
- **Result**: Double-free, refcount goes negative

### 2. Concurrent Switch Events
If multiple SwitchStreamProducerEvents fire rapidly:
- First switch gets `src` reference
- Second switch also gets `src` reference
- Both call `gst_object_unref(src)`
- **Result**: Double-unref

### 3. Pipeline Destroyed While Switching
If StopStreamEvent fires during switch:
- Switch handler has reference to `src`
- Stop handler destroys entire pipeline
- Switch handler tries to unref already-freed `src`
- **Result**: Use-after-free

## Potential Fixes

### Fix A: Add Mutex to Prevent Concurrent Operations
```cpp
// In switch handler, add:
static std::mutex switching_mutex;
std::lock_guard<std::mutex> lock(switching_mutex);
```

### Fix B: Check Pipeline State Before Switch
```cpp
// Before switching:
if (GST_STATE(pipeline.get()) != GST_STATE_PLAYING) {
    logs::log(logs::warning, "Ignoring switch - pipeline not in PLAYING state");
    return;
}
```

### Fix C: Disable Pause During Lobbies
Lobbies are meant to persist, so PauseStreamEvent shouldn't fire for lobby sessions:
```cpp
// In control.cpp, check if session is in a lobby before firing pause
if (!session_is_in_lobby(client_session->session_id)) {
    event_bus->fire_event(immer::box<PauseStreamEvent>(...));
}
```

## After Wolf Rebuild Completes

Once `./stack rebuild-wolf` finishes:

1. Wolf will restart with new logging
2. Run the monitoring script
3. Test lobby switching
4. When hang occurs, diagnostic logs will show exact sequence
5. Review logs to identify which hypothesis is correct
6. Implement targeted fix

## Current Status

- Wolf build in progress with diagnostic logging
- Monitor script ready
- Waiting for rebuild to complete
- Then ready for testing with you

The diagnostic logs will definitively show whether it's a race condition, concurrent access, or something else!
