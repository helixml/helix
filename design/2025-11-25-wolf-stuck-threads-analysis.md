# Wolf Stuck Threads Analysis - 2025-11-25

## Summary

Wolf crashed with the error:
```
03:56:01.307829020 ERROR | Unhandled exception: The state of the promise has already been set.
```

This was preceded by PulseAudio connection termination errors and resulted in 5/10 monitored threads getting stuck.

## Root Cause Analysis

### The Bug: Double-setting a std::promise

**Location:** `/wolf/src/core/src/platforms/linux/pulseaudio/pulse.cpp:67-91`

```cpp
pa_context_set_state_callback(
    ctx,
    [](pa_context *ctx, void *data) {
      auto state = (Server *)data;

      switch (pa_context_get_state(ctx)) {
      case PA_CONTEXT_READY:
        logs::log(logs::debug, "[PULSE] Pulse connection ready");
        state->on_ready.set_value(true);  // FIRST set_value
        break;
      case PA_CONTEXT_TERMINATED:
        logs::log(logs::debug, "[PULSE] Terminated connection");
        state->on_ready.set_value(false); // SECOND set_value - BOOM!
        break;
      case PA_CONTEXT_FAILED:
        logs::log(logs::debug, "[PULSE] Context failed");
        state->on_ready.set_value(false); // SECOND set_value - BOOM!
        break;
      // ...
      }
    },
    state.get());
```

**The Problem:**
- A `std::promise` can only have its value set ONCE
- The callback can be triggered multiple times during the lifecycle:
  1. `PA_CONTEXT_READY` → `set_value(true)` ✓
  2. Later, `PA_CONTEXT_TERMINATED` or `PA_CONTEXT_FAILED` → `set_value(false)` 💥

When PulseAudio disconnects (as seen in the logs), the state transitions from READY → FAILED, triggering the second `set_value()` which throws an exception.

### Crash Sequence

1. **03:48:09** - Multiple streaming sessions running normally
2. **03:56:01** - PulseAudio connection terminated (likely sandbox container audio issue)
   ```
   pulse pulsesrc.c:371:gst_pulsesrc_is_dead:<pulsesrc0> error: Disconnected: Connection terminated
   [PULSE] Context failed
   [GSTREAMER] Pipeline error: Disconnected: Connection terminated
   ```
3. **03:56:01** - Wolf's PulseAudio state callback fires with `PA_CONTEXT_FAILED`
4. **03:56:01** - `state->on_ready.set_value(false)` throws because promise already satisfied
   ```
   Unhandled exception: The state of the promise has already been set.
   ```
5. **03:56:01** - Exception causes thread to terminate abnormally
6. **03:56:01** - Wolf socket closes, but watchdog keeps running
7. **03:57:00+** - Watchdog reports: `System degraded: 5/10 threads stuck`

### Thread States at Time of Investigation

From GDB analysis (`design/2025-11-25-wolf-stuck-threads-debug.txt`):
- **Thread 5 (LWP 4567)** - Stuck on `__lll_lock_wait_private` (mutex contention)
- Multiple GStreamer pipeline threads terminated abnormally
- HTTP/HTTPS servers still running but unable to process requests

## Recommended Fixes

### 1. Use std::atomic<bool> instead of std::promise for connection state

```cpp
// Instead of:
std::promise<bool> on_ready;
std::future<bool> on_ready_fut;

// Use:
std::atomic<bool> is_ready{false};
std::atomic<bool> has_failed{false};
std::condition_variable ready_cv;
std::mutex ready_mutex;
```

### 2. Guard against double-setting with a flag

```cpp
std::atomic<bool> promise_set{false};

case PA_CONTEXT_READY:
  if (!promise_set.exchange(true)) {
    state->on_ready.set_value(true);
  }
  break;
case PA_CONTEXT_FAILED:
  if (!promise_set.exchange(true)) {
    state->on_ready.set_value(false);
  }
  break;
```

### 3. Catch the exception and handle gracefully

```cpp
try {
  state->on_ready.set_value(false);
} catch (const std::future_error& e) {
  logs::log(logs::debug, "[PULSE] Promise already set, ignoring state change");
}
```

### 4. Add process supervision in sandbox container

Even with this fix, Wolf should be automatically restarted on crash:
- Add supervisor script for Wolf, Moonlight Web, and dockerd
- Use s6-overlay or simple bash loop with restart logic
- Report crashes to health endpoint

## Related Files

- `wolf/src/core/src/platforms/linux/pulseaudio/pulse.cpp` - Bug location
- `wolf/src/moonlight-server/wolf.cpp` - Main Wolf process, watchdog
- `design/2025-11-25-wolf-stuck-threads-debug.txt` - Full GDB thread dump

## Applied Fix

The fix has been applied to `/wolf/src/core/src/platforms/linux/pulseaudio/pulse.cpp`:

```cpp
struct Server {
  pa_context *ctx;
  pa_mainloop *loop;
  boost::promise<bool> on_ready;
  boost::future<bool> on_ready_fut;
  std::atomic<bool> promise_set{false};  // Guard against double-setting promise
};

// In the callback:
case PA_CONTEXT_READY:
  logs::log(logs::debug, "[PULSE] Pulse connection ready");
  // Only set promise once - state can transition READY -> FAILED
  if (!state->promise_set.exchange(true)) {
    state->on_ready.set_value(true);
  }
  break;
case PA_CONTEXT_FAILED:
  logs::log(logs::debug, "[PULSE] Context failed");
  // Only set promise once - avoid "promise already satisfied" exception
  if (!state->promise_set.exchange(true)) {
    state->on_ready.set_value(false);
  } else {
    logs::log(logs::warning, "[PULSE] Connection failed after initial ready state");
  }
  break;
```

## Remaining Action Items

1. [x] Fix the double-set promise bug in pulse.cpp ✓
2. [x] Rename `./stack rebuild-wolf` to `./stack build-wolf` for consistency ✓
3. [x] Add automatic restart logic for Wolf in sandbox container ✓
4. [ ] Add PulseAudio reconnection logic instead of failing permanently
5. [x] Add timeout to API calls to Wolf (Go client already has 60s timeout) ✓
6. [x] Make Wolf socket resilient to exceptions (io_context restart loop) ✓
7. [x] Add socket health check to watchdog (detect dead API) ✓
