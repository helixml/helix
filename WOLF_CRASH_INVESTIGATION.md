# Wolf Random Video Hang Investigation - Design Document

## Problem Statement

Wolf (upstream wolf-ui branch) randomly hangs when switching video feeds between lobbies, occurring approximately 1 in 10 connection attempts. When the hang occurs:

- Video feed freezes (no frames delivered to client)
- Wolf process becomes zombie (cannot be killed gracefully)
- GStreamer logs flood with refcount errors: `gst_mini_object_unref: assertion 'GST_MINI_OBJECT_REFCOUNT_VALUE (mini_object) > 0' failed`
- Wolf container must be force-removed (`docker rm -f`)

## Current Environment

**Wolf Version**:
- Repository: `/home/luke/pm/wolf`
- Branch: `wolf-ui` (upstream games-on-whales/wolf)
- Our modifications: Only auto-pairing PIN support (57321eb)
- Base functionality: Upstream wolf-ui lobbies implementation

**Observed Behavior**:
- **Success rate**: ~90% (9/10 lobby joins work fine)
- **Failure rate**: ~10% (1/10 hangs with zombie process)
- **Log volume**: 120,884 lines in 10 minutes when hung
- **Primary error**: GStreamer mini_object refcount assertion failures

**Working Configuration**:
- Native Moonlight clients: 100% success (macOS, iPad, iOS, Linux)
- moonlight-web from external machine: 100% success
- Direct Wolf-UI lobby: Works reliably
- Switching between lobbies: 10% failure rate

## Technical Background

### Wolf-UI Lobbies Architecture

**Lobby System**:
- Each lobby has its own Wayland compositor and GStreamer video producer pipeline
- Wolf-UI session connects to Wolf and can switch between multiple lobbies
- When switching lobbies, Wolf sends `SwitchStreamProducerEvents` to change video source
- Video switching uses GStreamer `interpipe` to route different sources to active stream

**Key Components**:
1. **Video Producer Pipeline** (per lobby):
   - `waylanddisplaysrc` → captures from lobby's Wayland compositor
   - `interpipesink` → publishes to named pipe `{session_id}_video`

2. **Video Consumer Pipeline** (per client):
   - `interpipesrc listen-to={session_id}_video` → subscribes to active lobby
   - Encoder → RTP → UDP to client

3. **Switching Mechanism**:
   - Client joins lobby via `/api/v1/lobbies/join` with PIN
   - Wolf fires `SwitchStreamProducerEvents`
   - `interpipesrc` changes `listen-to` parameter to new lobby's pipe
   - Old lobby pipeline should continue running (for other clients)

### GStreamer Reference Counting

**How It Should Work**:
- Each GstMiniObject (buffer, event, etc.) has a reference count
- `gst_mini_object_ref()` increments count
- `gst_mini_object_unref()` decrements count
- When count reaches 0, object is freed
- **Assertion failure** means trying to unref an object with refcount already at 0

**Common Causes**:
1. Double-free: Code unrefs the same object twice
2. Ownership bug: Wrong part of code thinks it owns the object
3. Race condition: Two threads unref simultaneously
4. Pipeline destruction during active processing

## Evidence Collected

### From Logs

**When Working** (external client):
```
[RTP] Received ping from 172.19.0.16:36639 (20 bytes)
[RTP] audio from 172.19.0.16:36639
[RTP] video from 172.19.0.16:34483
```

**When Hung** (zombie process):
```
(wolf:1): GStreamer-CRITICAL **: 15:43:13.153: gst_mini_object_unref: assertion 'GST_MINI_OBJECT_REFCOUNT_VALUE (mini_object) > 0' failed
```
Repeated thousands of times per second.

**Wolf Process State**:
```
Error: cannot stop container: PID 4032621 is zombie and can not be killed
```

### Code Locations

**Pause Handlers** (streaming.cpp):
```cpp
auto pause_handler = event_bus->register_handler<immer::box<events::PauseStreamEvent>>(
    [sess_id, pipeline](const immer::box<events::PauseStreamEvent> &ev) {
      if (ev->session_id == sess_id) {
        logs::log(logs::debug, "[GSTREAMER] Pausing pipeline: {}", sess_id);
        // Sets pipeline to NULL state
      }
    });
```

**Lobby Switching** (lobbies.cpp):
```cpp
state_->app_state->event_bus->fire_event(
    immer::box<events::SwitchStreamProducerEvents>{
        .session_id = session->session_id,
        .interpipe_src_id = lobby->id
    });
```

## Hypothesis Tree

### Hypothesis 1: Race Condition During Pipeline Switching
**Likelihood**: HIGH

**Theory**: When switching lobbies, the following happens simultaneously:
1. Old lobby's video pipeline is still producing frames
2. `interpipesrc` switches to listen to new lobby
3. `PauseStreamEvent` fires for old lobby
4. Pipeline tries to set to NULL state while frames are in flight
5. Buffers get freed while still being processed
6. Refcount goes negative → assertion failures

**Evidence**:
- Intermittent nature (race conditions are timing-dependent)
- Only happens during switching (not static streaming)
- Refcount errors only appear during problematic switches

**Test**: Introduce artificial delay between switching lobbies

### Hypothesis 2: Pause Handler Double-Unref Bug
**Likelihood**: MEDIUM

**Theory**: The pause handlers (registered in streaming.cpp lines 302, 401) might:
- Be called multiple times for the same event
- Hold references to already-freed pipelines
- Not properly unregister when pipeline is destroyed

**Evidence**:
- Two separate pause handlers (video and audio)
- Event handlers persist across pipeline lifecycle
- No visible handler cleanup code

**Test**: Add logging to pause handlers to see if called repeatedly

### Hypothesis 3: interpipesrc Buffer Handoff Issue
**Likelihood**: MEDIUM

**Theory**: When `interpipesrc` switches sources:
- Old lobby still has buffers in `interpipesink`
- New lobby hasn't started sending yet
- `interpipesrc` tries to flush old buffers while switching
- Buffer ownership becomes unclear during transition

**Evidence**:
- Error message: "segment format mismatched, ignore"
- interpipe is designed for switching but might have edge cases
- Timing-dependent failure pattern

**Test**: Monitor interpipe buffer counts during switch

### Hypothesis 4: EGL/GL Context Sharing Bug
**Likelihood**: LOW (but mentioned in commit 65d596b)

**Theory**: Multiple lobbies sharing same GPU context:
- Each lobby creates GL context for waylanddisplaysrc
- Context cleanup during switching causes issues
- OpenGL resources freed in wrong order

**Evidence**:
- Commit message mentions "GL cleanup fix"
- Using helixml fork of gst-wayland-display
- NVIDIA-specific issues mentioned

**Test**: Test with `WOLF_USE_ZERO_COPY=TRUE` vs `FALSE`

## Investigation Plan

### Phase 1: Reproduce Reliably
**Goal**: Create automated test that can trigger the hang consistently

**Steps**:
1. Use stress test script: `/home/luke/pm/helix/test-lobby-join-stress.sh`
2. Automate lobby join/leave cycles (20+ iterations)
3. Monitor for refcount error spikes
4. Identify conditions that trigger hang

**Success Criteria**: Can reproduce hang in <50 attempts

### Phase 2: Isolate the Component
**Goal**: Determine which component has the bug

**Tests**:
1. **Test A**: Join lobby WITHOUT switching (stay in lobby)
   - Does hang still occur?
   - If no → switching logic is the problem
   - If yes → general streaming bug

2. **Test B**: Switch between lobbies WITHOUT video streaming
   - Create lobbies but don't connect client
   - Switch programmatically via API
   - Does refcount error appear?
   - If no → client connection/disconnection triggers it
   - If yes → lobby switching itself has bug

3. **Test C**: Stream to single lobby for extended period
   - No switching, just stream continuously
   - Does it eventually hang?
   - If no → confirms switching is the trigger
   - If yes → memory leak accumulates

**Expected Outcome**: Narrow down to specific operation

### Phase 3: Add Diagnostic Logging
**Goal**: Capture exact state when bug triggers

**Logging Points**:
1. `PauseStreamEvent` handler entry/exit
2. `SwitchStreamProducerEvents` handler
3. interpipesrc `listen-to` parameter changes
4. GStreamer pipeline state changes
5. Buffer ref/unref in custom sink

**Implementation**:
```cpp
// In streaming.cpp pause handler:
logs::log(logs::warning, "[DEBUG] PauseStreamEvent fired for session {}, pipeline state: {}",
          sess_id, gst_element_state_get_name(GST_STATE(pipeline.get())));
```

**Data to Collect**:
- Timing: When does first refcount error appear?
- State: What pipeline state transitions happened just before?
- Concurrency: Are multiple pause events firing simultaneously?

### Phase 4: Code Review - Event Handler Lifecycle
**Goal**: Verify handlers are properly managed

**Check**:
1. Are pause handlers unregistered when pipelines are destroyed?
2. Do handlers hold weak or strong references to pipelines?
3. Can handlers fire after pipeline destruction?

**Files to Review**:
- `src/moonlight-server/streaming/streaming.cpp` (pause handlers)
- `src/moonlight-server/sessions/lobbies.cpp` (switch logic)
- `src/core/include/events.hpp` (event bus implementation)

**Questions**:
- What happens to registered handlers when event_bus is destroyed?
- Are pipeline shared_ptrs captured correctly in lambdas?
- Could handlers outlive the pipelines they reference?

### Phase 5: Potential Fixes

**Fix A: Add Pipeline Switching Lock**
```cpp
// In lobbies.cpp before firing SwitchStreamProducerEvents:
std::lock_guard lock(switching_mutex);

// Wait for old pipeline to flush buffers
old_pipeline_flush_complete.wait();

// Then switch
fire_event(SwitchStreamProducerEvents{...});
```

**Fix B: Disable Pause During Switch**
```cpp
// In streaming.cpp pause handler:
if (switching_in_progress.load()) {
    logs::log(logs::debug, "Ignoring pause during lobby switch");
    return;
}
```

**Fix C: Explicit Handler Cleanup**
```cpp
// Store handler IDs when registering
auto pause_handler_id = event_bus->register_handler(...);

// In pipeline cleanup:
event_bus->unregister_handler(pause_handler_id);
```

**Fix D: Report to Upstream**
If we can't fix locally:
- Document reproduction steps
- Create minimal test case
- Submit issue to games-on-whales/wolf with full details
- Wait for upstream fix or apply workaround

## Reproduction Steps

### Manual Reproduction
1. Connect to Wolf-UI via moonlight-web: `http://node01.lukemarsden.net:8081`
2. Launch Wolf UI app (app_id from `/api/apps/0`)
3. Inside Wolf-UI, create or join a Zed external agent lobby
4. Enter PIN to join
5. Observe if video feed hangs (1/10 chance)
6. If hung, check Wolf logs for refcount errors

### Automated Reproduction
```bash
cd /home/luke/pm/helix

# Run stress test
./test-lobby-join-stress.sh

# Monitor Wolf for zombie state
watch 'docker inspect helix-wolf-1 | grep "\"Status\":"'

# If zombie detected:
docker rm -f helix-wolf-1
docker compose -f docker-compose.dev.yaml up -d wolf
```

## Debugging Tools

### Wolf Log Analysis
```bash
# Count refcount errors per minute
docker compose -f docker-compose.dev.yaml logs wolf --since 1m 2>&1 | grep -c "gst_mini_object_unref"

# Find first refcount error (when bug starts)
docker compose -f docker-compose.dev.yaml logs wolf 2>&1 | strings | grep -B50 "gst_mini_object_unref" | head -60

# Check if Wolf is zombie
docker inspect helix-wolf-1 | jq '.[0].State'
```

### GStreamer Pipeline State
```bash
# Inside Wolf container (if responsive)
docker exec helix-wolf-1 gst-launch-1.0 --gst-debug=3 interpipesrc listen-to=9950542253436598280_video ! fakesink

# Check pipeline state for a session
docker exec helix-wolf-1 sh -c 'GST_DEBUG=3 gst-inspect-1.0 interpipesink'
```

### Event Bus Activity
```bash
# Enable event bus logging in Wolf
# Set RUST_LOG=wolf::core::events=trace in docker-compose

# Check event firing frequency
docker compose -f docker-compose.dev.yaml logs wolf | grep -E "PauseStreamEvent|SwitchStreamProducerEvents|JoinLobbyEvent"
```

## Metrics to Track

During stress testing, measure:
- **Hang frequency**: X hangs per Y attempts
- **Time to hang**: How long streaming before hang occurs
- **Refcount error rate**: Errors per second when hung
- **CPU usage**: Wolf CPU % before/during hang
- **Memory growth**: Wolf RSS memory trend
- **Lobby switch count**: How many switches before hang

## Success Criteria

**Investigation Complete When**:
1. Can reliably reproduce hang (>80% within 100 attempts)
2. Identified exact code path causing refcount errors
3. Root cause documented with evidence

**Bug Fixed When**:
1. Can complete 100+ lobby switches without hang
2. Zero GStreamer refcount errors
3. Wolf process never becomes zombie
4. No memory leaks observed

## Timeline

- **Phase 1**: 1-2 hours (automated reproduction)
- **Phase 2**: 2-3 hours (component isolation)
- **Phase 3**: 1-2 hours (diagnostic logging)
- **Phase 4**: 3-4 hours (code review)
- **Phase 5**: Variable (depends on fix complexity)

**Total Estimate**: 8-15 hours for full investigation and fix

## Related Files

**Wolf Source**:
- `src/moonlight-server/streaming/streaming.cpp` - Pause handlers
- `src/moonlight-server/sessions/lobbies.cpp` - Lobby join/leave/switch
- `src/moonlight-server/api/endpoints.cpp` - API handlers
- `src/core/include/events.hpp` - Event bus

**Wolf-UI Source**:
- `src/Resources/WolfAPI/v1/Lobbies.cs` - Join/leave API calls
- `src/Scenes/Main/Body/Apps/App.cs` - App launching logic
- `src/Scenes/Main/Body/Lobby/Lobby.cs` - Lobby UI and PIN entry

**Helix**:
- `test-lobby-join-stress.sh` - Automated reproduction script
- `wolf/config.toml` - Wolf app configuration

## Open Questions

1. **Is the bug in Wolf or gst-wayland-display?**
   - Commit 65d596b mentions "GL cleanup fix" in gst-wayland-display fork
   - Are we using the right fork/branch?

2. **Does the bug occur with single client or only multiple?**
   - Wolf-UI connects to one lobby
   - Then switches to join another lobby
   - Are both lobbies' pipelines running simultaneously?

3. **Is there a memory leak accumulating?**
   - Does the bug happen more frequently after N hours of uptime?
   - Does Wolf memory usage grow over time?

4. **Are pause handlers even needed for lobby switching?**
   - Lobbies should stay alive when clients disconnect
   - Maybe pause handlers should be removed entirely?

5. **Can we reproduce with native Moonlight client?**
   - Connect native client to Wolf-UI
   - Switch lobbies from within Wolf-UI
   - Does native client also trigger the bug?

## Upstream Reporting Template

If we determine this is an upstream bug:

```markdown
**Title**: Random GStreamer refcount errors and zombie process when switching lobbies

**Environment**:
- Wolf version: wolf-ui branch (latest)
- OS: Linux (Docker)
- GPU: NVIDIA RTX 2000 Ada
- GStreamer: 1.26.2

**Description**:
When using Wolf-UI to switch between lobbies, approximately 1 in 10 attempts results in:
- GStreamer refcount assertion failures (gst_mini_object_unref)
- Wolf process becomes zombie (unkillable)
- Video feed hangs

**Reproduction**:
1. Start Wolf-UI lobby
2. Create second lobby (Zed external agent)
3. Join second lobby with PIN
4. ~10% of joins cause hang with refcount errors

**Expected**: Smooth lobby switching
**Actual**: Random hangs requiring force-kill

**Logs**: [attach logs showing refcount errors]

**Impact**: Makes multi-lobby feature unreliable for production use
```

## Workarounds

### Temporary Workarounds (Current)

**When Wolf Hangs**:
```bash
docker rm -f helix-wolf-1
docker compose -f docker-compose.dev.yaml up -d wolf
```

**Reduce Log Volume**:
- Set `WOLF_LOG_LEVEL=INFO` (not TRACE/DEBUG)
- Set `GST_DEBUG=1` (not 2/3)
- Prevents log files from filling disk

**Avoid Switching**:
- Use separate Wolf-UI sessions for each lobby
- Don't switch between lobbies in same session
- Not ideal but avoids trigger

### Long-term Solutions

**Solution A**: Fix upstream and wait for merge
**Solution B**: Maintain our own wolf-ui fork with fix
**Solution C**: Disable multi-lobby feature (use separate sessions)
**Solution D**: Report and work around (document as known issue)

## Next Steps

**Immediate** (next session):
1. Run stress test to get reproduction rate
2. Enable diagnostic logging in Wolf streaming.cpp
3. Capture full event sequence during hang
4. Review pause handler registration/cleanup

**Short-term** (this week):
1. Test with native Moonlight client (rule out moonlight-web)
2. Review gst-wayland-display fork for issues
3. Check if bug exists in upstream stable branch
4. Determine if reportable or Helix-specific

**Long-term** (ongoing):
1. Monitor bug frequency in production use
2. Track if fixed in upstream updates
3. Consider maintaining fork if necessary

## Success Metrics

- ✅ Streaming works from external machines (ACHIEVED)
- ✅ Mouse/keyboard input works in embedded client (ACHIEVED)
- ✅ Screenshots work reliably (ACHIEVED)
- ✅ Zed bidirectional messaging works (ACHIEVED)
- ⏳ Lobby switching 100% reliable (IN PROGRESS - currently 90%)
- ⏳ Zero GStreamer refcount errors (IN PROGRESS - investigating)
- ⏳ Wolf never becomes zombie (IN PROGRESS - needs fix)

## References

- Wolf-UI Lobbies PR: https://github.com/games-on-whales/wolf/pull/xxx (find PR number)
- GStreamer interpipe: https://github.com/RidgeRun/gst-interpipe
- GStreamer buffer lifecycle: https://gstreamer.freedesktop.org/documentation/plugin-development/advanced/allocation.html
- Helix Wolf integration: `/home/luke/pm/helix/api/pkg/external-agent/wolf_executor.go`
