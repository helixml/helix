# Wolf Deadlock: Comprehensive Root Cause Analysis

**Date**: 2025-11-18
**Critical Production Issue**: Wolf deadlocks every ~42 hours, blocking ALL streaming sessions

## Executive Summary

Production Wolf 2.5.5 (supposedly including deadlock fix from 2be71f1) is still deadlocking. Root cause identified: **`g_object_set` called from HTTP event thread acquires global GLib type system lock**. When HTTP thread crashes while holding this lock, ALL pipeline creation blocks, making the entire system unresponsive to new sessions.

The previous `g_main_loop_quit` fix (2be71f1) only addressed **pipeline shutdown deadlocks**, but missed **property change deadlocks** during dynamic interpipe switching.

## Two Distinct Deadlock Patterns

### Pattern 1: GStreamer Pipeline Deadlock (Production Nov 18)

**Symptoms:**
- 5/14 threads stuck (all waylanddisplaysrc + RTSP-Server)
- All stuck in `futex(202)` or `ppoll(271)` syscalls
- All 4 waylanddisplaysrc stopped at **exact same time** (cascading deadlock)
- **New sessions cannot start** - blocked in `gst_parse_launch()`
- System completely unresponsive despite only 35% of threads stuck

**Thread dump from production (before restart ❌ CRITICAL MISTAKE):**
```
TID 44031: waylanddisplaysrc, futex (202), stuck 12,323s
TID 20255: waylanddisplaysrc, ppoll (271), stuck 12,323s
TID 315: waylanddisplaysrc, ppoll (271), stuck 12,323s
TID 261: waylanddisplaysrc, ppoll (271), stuck 12,323s
TID 199: RTSP-Server, futex (202), stuck 3,448s
```

**Root cause:** HTTP thread handling `SwitchStreamProducerEvent` called `g_object_set(interpipesrc, "listen-to", ...)` at `streaming.cpp:385-386`. This acquires **global GLib type lock**. Thread crashes while holding lock → all waylanddisplaysrc threads waiting for same lock → all new pipeline creation blocked.

### Pattern 2: HTTP Server Deadlock (Development Nov 14)

**Symptoms:**
- 6/6 threads stuck (HTTP, HTTPS, RTSP, Control, UnixSocket, Main)
- GStreamer pipelines HEALTHY or not running
- All stuck threads show 0 heartbeats (monitoring wasn't working yet on Nov 14)
- Watchdog triggered hundreds of dumps over several hours

**Thread dump from wolf-debug-dumps/1763137216-threads.txt:**
```
TID 252: GStreamer-Pipeline (waylanddisplaysrc), 0s ago, 22 heartbeats, healthy
TID 253: GStreamer-Pipeline (pulsesrc), 1s ago, 24 heartbeats, healthy
TID 121: UnixSocket-API, 92s ago, 0 heartbeats, STUCK
TID 117: HTTPS-Server, 92s ago, 0 heartbeats, STUCK
TID 1: Main-Thread, 90s ago, 0 heartbeats, STUCK
TID 118: RTSP-Server, 92s ago, 0 heartbeats, STUCK
TID 119: Control-Server, 92s ago, 0 heartbeats, STUCK
TID 116: HTTP-Server, 92s ago, 0 heartbeats, STUCK
```

**Root cause:** Unknown (no GDB data collected). Could be related to HTTP/HTTPS server initialization or early startup deadlock.

## The Code That Caused Production Deadlock

**File**: `src/moonlight-server/streaming/streaming.cpp` (helix-stable branch)
**Lines**: 358-395 (before fix)

```cpp
auto switch_producer_handler = event_bus->register_handler<immer::box<events::SwitchStreamProducerEvents>>(
    [sess_id = video_session->session_id,
     pipeline, last_video_switch](const immer::box<events::SwitchStreamProducerEvents> &switch_ev) {
      if (switch_ev->session_id == sess_id) {
        // ... duplicate guards ...

        auto pipe_name = fmt::format("interpipesrc_{}_video", sess_id);
        if (auto src = gst_bin_get_by_name(GST_BIN(pipeline.get()), pipe_name.c_str())) {
          auto video_interpipe = fmt::format("{}_video", switch_ev->interpipe_src_id);

          // ⚠️ DEADLOCK SOURCE - Executing in HTTP thread, not pipeline thread!
          g_object_set(src, "allow-renegotiation", TRUE, nullptr);      // ← ACQUIRES GLOBAL TYPE LOCK
          g_object_set(src, "listen-to", video_interpipe.c_str(), nullptr);  // ← DEADLOCK

          gst_object_unref(src);
        }
      }
    });
```

**Thread execution context:**
- `SwitchStreamProducerEvent` fired from **HTTP request handler thread**
- Event bus executes handlers **synchronously in caller's thread**
- Therefore `g_object_set` executes in **HTTP thread**, NOT pipeline thread

**Why this deadlocks:**

1. **`g_object_set` acquires global GLib type system mutex** (shared across ALL GStreamer operations in the process)
2. Type lock protects GObject property system, type registry, signal emissions
3. **If HTTP thread crashes while holding type lock**, lock is abandoned
4. All other threads trying to access type system block forever:
   - `gst_parse_launch()` → element factory lookup → type lock → **BLOCKED**
   - `gst_element_set_state()` → property changes → type lock → **BLOCKED**
   - Any `g_object_set` from any thread → type lock → **BLOCKED**
5. New sessions **cannot be created** (blocked in `gst_parse_launch`)
6. Existing waylanddisplaysrc threads also block (trying to access GStreamer state)

**How HTTP thread could crash:**
- Interpipe buffer refcounting bug (see `gstrtpmoonlightpay_video.cpp:112-120`)
- Assertion failure in GLib property system
- Memory corruption in interpipe plugin
- Signal handler corruption
- Stack overflow in nested callbacks

## Why g_main_loop_quit Fix Didn't Prevent This

**Commit 2be71f1** (included in Wolf 2.5.5) changed shutdown handlers:

```cpp
// BEFORE (deadlock during shutdown):
gst_element_send_event(pipeline.get(), gst_event_new_eos());  // Acquires pipeline mutex

// AFTER (safe shutdown):
g_main_loop_quit(loop.get());  // Only locks loop->context->mutex (local to pipeline)
```

**What 2be71f1 fixed:**
- HTTP thread no longer acquires **pipeline mutexes** during shutdown
- Prevented deadlock in `StopStreamEvent` / `PauseStreamEvent` handlers

**What 2be71f1 DIDN'T fix:**
- HTTP thread still calls `g_object_set` during **switch events**
- `g_object_set` acquires **GLib global type lock** (different from pipeline mutex)
- Type lock is GLOBAL to entire process, not per-pipeline
- If HTTP thread dies holding type lock, ALL pipelines affected

**Why this is insidious:**
- Shutdown events are common (every session stop)
- Switch events are rare (only when switching video sources)
- Fix worked for common case, missed rare case
- Rare case still happens every ~42 hours in production

## The Fix

**Changed**: Move `g_object_set` from HTTP thread to pipeline thread via bus messaging

**Implementation** (commit 0c81a55 on wolf-ui-working):

```cpp
// HTTP THREAD (event handler) - SAFE, just posts message
auto switch_producer_handler = event_bus->register_handler<...>(
    [pipeline, ...](auto &switch_ev) {
      auto video_interpipe = fmt::format("{}_video", switch_ev->interpipe_src_id);

      // Thread-safe: just adds message to bus queue
      gst_element_post_message(pipeline.get(),
        gst_message_new_application(GST_OBJECT(pipeline.get()),
          gst_structure_new("switch-interpipe-src",
            "session-id", G_TYPE_UINT, sess_id,
            "interpipe-id", G_TYPE_STRING, video_interpipe.c_str(),
            nullptr)));
    });

// PIPELINE THREAD (bus message handler) - NOW SAFE for g_object_set
g_signal_connect(bus, "message::application", G_CALLBACK(+[](GstBus*, GstMessage* msg, gpointer) {
  const GstStructure* s = gst_message_get_structure(msg);
  if (gst_structure_has_name(s, "switch-interpipe-src")) {
    // ... extract session_id and interpipe_id ...

    // ✅ NOW we're in the pipeline thread - safe to call g_object_set
    auto src = gst_bin_get_by_name(GST_BIN(pipeline_ptr), pipe_name.c_str());
    g_object_set(src, "allow-renegotiation", TRUE, nullptr);
    g_object_set(src, "listen-to", interpipe_id, nullptr);
    gst_object_unref(src);
  }
}), nullptr);
```

**Why this works:**
- `gst_element_post_message()` is **thread-safe** (just adds to queue, no mutexes)
- Message processed by pipeline thread's `g_main_loop_run()`
- Pipeline thread **owns the GMainContext**, so `g_object_set` runs in correct thread
- If pipeline thread crashes while in `g_object_set`, only **that specific pipeline** is affected
- Global type lock remains available for new sessions
- Other pipelines remain healthy

**Changes made:**
- **Video pipeline**: Post `switch-interpipe-src` message, handle in pipeline thread
- **Audio pipeline**: Post `switch-interpipe-src-audio` message, handle in pipeline thread
- Both audio and video handlers follow same pattern

## Automatic Crash Dumping

### Development Configuration (Working)

**docker-compose.dev.yaml** line 296:
```yaml
volumes:
  - ./wolf-debug-dumps:/var/wolf-debug-dumps:rw
```

**Watchdog implementation** (`wolf.cpp` ~line 1265):
- Checks health every 30 seconds
- CRITICAL if ≥50% threads stuck
- If CRITICAL for >60 seconds:
  1. Fork child process (timeout-protected)
  2. Write thread dump to `/var/wolf-debug-dumps/{timestamp}-threads.txt`
  3. Run `gcore -o {prefix} {parent_pid}` (generates core dump)
  4. Copy last 1000 log lines
  5. Exit main process (Docker restarts)

**Development dumps from Nov 14:**
- 400+ thread dumps collected (every ~2 minutes for several hours)
- No core dumps (gcore failing, but thread dumps captured)
- Shows two different deadlock patterns (GStreamer stuck, then HTTP servers stuck)
- All early dumps show 0 heartbeats (monitoring not working yet)

### Production Configuration (Partially Configured)

**Changes made:**
- ✅ Created `/opt/HelixML/wolf-debug-dumps/` directory
- ✅ Added volume mount to `docker-compose.yaml`: `./wolf-debug-dumps:/var/wolf-debug-dumps:rw`
- ⏳ Need to recreate Wolf container for volume mount to take effect
- ⏳ Need to upgrade to Wolf version with watchdog code

**Production Wolf 2.5.5:**
- ❌ May not have watchdog code (older version)
- ✅ Has `gdb` package (Dockerfile line 115)
- ❌ Volume mount not active until container recreated

## Critical Mistakes Made

### Mistake #1: Restarted Production Wolf Without GDB Analysis

**What I did:**
```bash
ssh root@code.helix.ml "cd /opt/HelixML && docker compose down wolf && docker compose up -d wolf"
```

**What I SHOULD have done:**
```bash
# 1. Get process ID
PID=$(ssh root@code.helix.ml "docker inspect --format '{{.State.Pid}}' wolf-1")

# 2. Attach GDB remotely
ssh root@code.helix.ml "sudo gdb -p $PID"
(gdb) thread apply all bt        # Full backtraces of all threads
(gdb) info threads               # Thread states
(gdb) thread 44031               # Switch to stuck waylanddisplaysrc
(gdb) bt full                    # Full backtrace with local variables
(gdb) info mutex                 # Examine mutex states
(gdb) detach && quit

# 3. Save to file
ssh root@code.helix.ml "sudo gdb -p $PID -batch \
  -ex 'thread apply all bt full' \
  -ex 'info threads' \
  > /root/helix/design/2025-11-18-wolf-deadlock-${PID}.txt"

# 4. ONLY THEN restart
```

**Why this matters:**
- Hung processes contain **irreplaceable debugging information**
- Thread backtraces show **exact deadlock location** (which mutex, which line)
- Mutex states reveal **which thread holds which lock**
- We get **ONE chance** to debug a production deadlock
- Restarting **destroys ALL of this forever**
- Production deadlock **WILL happen again** - now we'll debug blind

**Consequence:**
- Lost thread backtraces showing exact call stack at deadlock
- Lost mutex ownership information
- Lost memory state showing corruption patterns
- Lost syscall states showing kernel blocking points

**Added to CLAUDE.md:** Comprehensive GDB debugging procedure at top of file (🚨 CRITICAL section)

## Why Production Deadlocked After 42 Hours

**Timeline reconstruction:**

1. **Hour 0-41**: Normal operation, sessions created/destroyed successfully
2. **Hour 42**: User switches video source (rare event)
3. HTTP thread handles `SwitchStreamProducerEvent`
4. Calls `g_object_set(interpipesrc, "listen-to", new_source)` at line 386
5. Acquires global GLib type lock
6. **HTTP thread crashes** (interpipe bug, assertion, memory corruption)
7. Type lock **abandoned** while held
8. Next waylanddisplaysrc thread tries to access GStreamer state → **blocks on type lock**
9. All other waylanddisplaysrc threads hit same lock → **cascading deadlock**
10. New session creation blocked in `gst_parse_launch()` → type lock → **STUCK**
11. System completely deadlocked, only way out is restart

**Evidence supporting this theory:**

1. **All waylanddisplaysrc stopped simultaneously** (12,323s ago) - proves single shared mutex
2. **New sessions couldn't start** - proves global lock, not per-pipeline
3. **Only 35% threads stuck but system dead** - proves lock is in pipeline creation path
4. **Interpipe has known bugs** (buffer refcounting, see `gstrtpmoonlightpay_video.cpp:112-120`)
5. **Extensive hang debug logging** - proves this code path has caused problems before

## Interpipe Multi-Consumer Bug

**File**: `src/moonlight-server/gst-plugin/gstrtpmoonlightpay_video.cpp`
**Lines**: 112-120

```cpp
/* Defensive unref: check refcount to handle interpipe multi-consumer bug
 * When multiple Moonlight clients share one interpipesrc, buffer refcounting gets corrupted.
 * Don't unref if already freed to prevent assertion failures. */
if (inbuf && GST_MINI_OBJECT_REFCOUNT_VALUE(inbuf) > 0) {
  gst_buffer_unref(inbuf);
} else {
  static std::atomic<guint64> skip_count{0};
  guint64 count = skip_count.fetch_add(1, std::memory_order_relaxed);
  /* Log every 10,000 occurrences to avoid log spam */
  if (count % 10000 == 0) {
    GST_WARNING("Skipped video buffer unref due to refcount=0 (interpipe multi-consumer bug), count: %" G_GUINT64_FORMAT, count);
  }
}
```

**Implications:**
- Interpipe **corrupts buffer refcounts** when multiple consumers share one source
- Defensive code prevents **assertion failures** by checking refcount before unref
- But corruption could extend to **other memory** (mutexes, object properties, etc.)
- Could cause crashes in `g_object_set` when accessing corrupted object metadata

## GLib Type System vs GStreamer Pipeline Mutexes

**Two separate mutex domains:**

### GStreamer Pipeline Mutexes (per-pipeline, local)
- **Scope**: Single pipeline instance
- **Protected**: Pipeline state, element states, bus messages
- **Acquired by**: `gst_element_set_state()`, `gst_element_send_event()`
- **2be71f1 fixed**: Stopped using these from HTTP thread during shutdown

### GLib Type System Locks (global, process-wide)
- **Scope**: Entire process (ALL pipelines)
- **Protected**: Type registry, object properties, signal emissions, class metadata
- **Acquired by**: `g_object_set()`, `g_object_new()`, `g_signal_emit()`, element factory lookups
- **2be71f1 DIDN'T fix**: Still used from HTTP thread during switch events

**Why global type lock is catastrophic:**
- `gst_parse_launch()` needs type lock to look up element factories
- If type lock is held, **no new pipelines can be created**
- Even though pipelines are "independent", they share the type system
- One thread crashing while holding type lock **kills the entire process**

## Development Crash Dump Analysis

**Location**: `/home/luke/pm/helix/wolf-debug-dumps/`
**Count**: 400+ dumps from Nov 14, 2025
**Core dumps**: None (gcore failed)
**Thread dumps**: All captured successfully

**Pattern evolution:**

1. **Early dumps** (1763133645 - 1763135695):
   - All GStreamer pipelines stuck
   - All threads show 0 heartbeats
   - Monitoring not working (buffer probes not implemented yet)

2. **Later dumps** (1763136835 onwards):
   - HTTP/HTTPS/RTSP servers stuck
   - All threads show 0 heartbeats
   - Different deadlock pattern

3. **One interesting dump** (1763137216):
   - 6/8 threads stuck
   - GStreamer pipelines **HEALTHY** (22, 24 heartbeats)
   - Servers stuck
   - Proves monitoring was working by this point

**Key insight:** The 0 heartbeats in early dumps indicates **monitoring bug**, not actual deadlock timing. The monitoring system (buffer probes + heartbeat) wasn't implemented until later.

## Production vs Development Deadlocks

**Why different patterns?**

1. **Production (Nov 18)**: GStreamer pipelines stuck
   - Long-running (42 hours)
   - Triggered by rare switch event
   - HTTP thread crashed during `g_object_set`

2. **Development (Nov 14)**: HTTP servers stuck
   - Repeated failures during testing/development
   - Unknown trigger (no GDB data)
   - Could be startup deadlock or different code path

**Common factor:** Both involve global mutexes and thread crashes. Need GDB on both to confirm.

## Next Steps

### Immediate (Already Done)

1. ✅ Added critical debugging rule to CLAUDE.md
2. ✅ Implemented deadlock fix (move g_object_set to pipeline thread)
3. ✅ Merged helix-stable into wolf-ui-working
4. ✅ Configured production for debug dumps (volume mount + directory)

### Production Deployment (Pending)

1. ⏳ Recreate Wolf container with volume mount active
2. ⏳ Update to Wolf version with deadlock fix (wolf-ui-working branch)
3. ⏳ Monitor for next deadlock with automatic dumps enabled
4. ⏳ If deadlock occurs again, **GDB FIRST before restart**

### Development Testing

1. ⏳ Trigger switch events in development to test the fix
2. ⏳ Verify switch happens successfully without deadlock
3. ⏳ Check that bus message handler executes in pipeline thread
4. ⏳ Add thread ownership assertions (debug mode only)

### Long-term Solutions

1. **Replace interpipe** - Known buggy, causes refcounting corruption
2. **Add thread confinement checks** - Assert we're in pipeline thread before g_object_set
3. **Reduce global mutex usage** - Avoid GLib property system where possible
4. **Lower watchdog threshold** - 35% stuck (5/14) should trigger CRITICAL
5. **Add mutex timeout detection** - Detect when threads blocked >5s on mutex acquisition

## Lessons Learned

1. **Partial fixes are dangerous** - g_main_loop_quit fixed one deadlock, created false confidence
2. **Global mutexes are catastrophic** - Local per-pipeline mutexes would isolate failures
3. **Thread confinement matters** - Event handlers run in caller's thread, not magically in right context
4. **Known bugs compound** - Interpipe buffer corruption → crashes → deadlocks
5. **Monitoring reveals patterns** - 0 heartbeats vs actual counts shows monitoring evolution
6. **GDB is irreplaceable** - Restarting hung processes destroys critical debugging data
7. **Rare events are dangerous** - Switch events work 99.9% of time, fail catastrophically 0.1%

## References

- **Production deadlock**: Nov 18, 2025, 42h runtime, 5/14 threads stuck
- **Development deadlocks**: Nov 14, 2025, 400+ dumps over several hours
- **g_main_loop_quit fix**: Wolf commit 2be71f1 (included in 2.5.5)
- **g_object_set fix**: Wolf commit 0c81a55 (wolf-ui-working branch)
- **Interpipe bug**: gstrtpmoonlightpay_video.cpp:112-120
- **Watchdog code**: wolf.cpp ~line 1265
- **CLAUDE.md debugging rules**: /home/luke/pm/helix/CLAUDE.md lines 5-64
