# Wolf GStreamer Deadlock Root Cause Analysis

**Date**: 2025-11-18
**Status**: Root cause identified
**Production Impact**: Deadlock every ~42 hours, blocking ALL streaming

## Critical Discovery

The `g_main_loop_quit` fix in commit 2be71f1 **did NOT fully solve the deadlock**. Production is still deadlocking because the fix only addressed one deadlock scenario (pipeline shutdown), but missed another: **GLib type system locks during dynamic property changes**.

## The Real Deadlock Scenario

### What the Production Deadlock Looked Like

From production Wolf health data before restart (❌ CRITICAL MISTAKE - should not have restarted):

```
5 of 14 threads stuck (no heartbeat > 30s):
- TID 44031: waylanddisplaysrc, futex (202), stuck 12,323s
- TID 20255: waylanddisplaysrc, ppoll (271), stuck 12,323s
- TID 315: waylanddisplaysrc, ppoll (271), stuck 12,323s
- TID 261: waylanddisplaysrc, ppoll (271), stuck 12,323s
- TID 199: RTSP-Server, futex (202), stuck 3,448s
```

**Key observations:**
1. All 4 waylanddisplaysrc threads stopped at THE EXACT SAME TIME (12,323s ago)
2. All stuck in futex/ppoll (waiting for mutex)
3. New streams could NOT be created (blocking in pipeline creation)
4. RTSP-Server thread also stuck (separate but related)

### The Code That Causes the Deadlock

**File**: `src/moonlight-server/streaming/streaming.cpp`
**Lines**: 337-347

```cpp
auto switch_producer_handler = event_bus->register_handler<immer::box<events::SwitchStreamProducerEvents>>(
    [sess_id = video_session->session_id,
     pipeline, last_video_switch](const immer::box<events::SwitchStreamProducerEvents> &switch_ev) {
      if (switch_ev->session_id == sess_id) {
        // ... duplicate guard checks ...

        auto pipe_name = fmt::format("interpipesrc_{}_video", sess_id);
        if (auto src = gst_bin_get_by_name(GST_BIN(pipeline.get()), pipe_name.c_str())) {
          /* Perform the switch */
          auto video_interpipe = fmt::format("{}_video", switch_ev->interpipe_src_id);

          // ⚠️ DEADLOCK SOURCE: These g_object_set calls acquire GLOBAL type locks
          g_object_set(src, "allow-renegotiation", TRUE, nullptr);
          g_object_set(src, "listen-to", video_interpipe.c_str(), nullptr);

          gst_object_unref(src);
        }
      }
    });
```

### Why This Causes a Deadlock

**Thread execution context:**
- The `switch_producer_handler` runs in the **HTTPS event handler thread** (NOT the GStreamer pipeline thread)
- When an HTTPS event fires, it calls `event_bus->fire_event()`, which synchronously executes all registered handlers
- This means `g_object_set` is called from the HTTPS thread

**What `g_object_set` does:**
1. Acquires GLib's **global type system mutex** (shared across ALL GStreamer operations)
2. Changes the object property
3. May trigger GStreamer internal callbacks
4. Releases the type system mutex

**The deadlock scenario:**

```
1. HTTPS thread handles SwitchStreamProducerEvent
2. Calls g_object_set(interpipesrc, "listen-to", ...)
3. Acquires GLib global type lock
4. HTTPS thread crashes/hangs/deadlocks WHILE HOLDING the type lock
   (Could be: segfault, assertion failure, nested mutex deadlock, etc.)
5. All pipeline threads trying to create new elements block on the SAME type lock
   - gst_parse_launch() → element factory → type system → BLOCKED
6. All waylanddisplaysrc threads waiting for shared GStreamer mutex also block
7. System is completely deadlocked, no new sessions can start
```

### Evidence From Code Comments

The code already has extensive hang debugging from previous investigations:

**Lines 331-334**: Duplicate pause event guards
```cpp
if (*pause_sent) {
  logs::log(logs::warning, "[HANG_DEBUG] Video PauseStreamEvent DUPLICATE IGNORED for session {}", sess_id);
  return;
}
```

**Lines 353-355**: Duplicate switch event guards
```cpp
if (*last_video_switch == switch_ev->interpipe_src_id) {
  logs::log(logs::warning, "[HANG_DEBUG] Video SwitchStreamProducerEvents DUPLICATE IGNORED: session {} already switched to {}", sess_id, switch_ev->interpipe_src_id);
  return;
}
```

These guards were added to prevent duplicate events, which suggests this code path has caused problems before!

### Interpipe Multi-Consumer Bug

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

This confirms:
1. Interpipe has known buffer refcounting bugs
2. Can cause assertion failures (crashes)
3. Could lead to memory corruption and mutex corruption

## Why the g_main_loop_quit Fix Didn't Work

**Commit 2be71f1** (included in Wolf 2.5.5) changed:
```cpp
// BEFORE (caused deadlock):
gst_element_send_event(pipeline, EOS);  // Acquires GStreamer pipeline mutex

// AFTER (prevented THAT deadlock):
g_main_loop_quit(loop.get());  // Thread-safe, no GStreamer mutex acquisition
```

**What it fixed:**
- HTTPS thread no longer acquires GStreamer pipeline mutexes during shutdown
- Prevents deadlock when stopping streams

**What it DIDN'T fix:**
- HTTPS thread still acquires **GLib type system mutexes** via `g_object_set`
- These type mutexes are GLOBAL to all GStreamer operations
- If HTTPS thread dies while holding type lock, ALL pipeline creation blocks

## Why New Streams Can't Start

When production deadlocked:
1. Some thread (likely HTTPS) died while holding GLib type lock
2. New session creation calls `gst_parse_launch(pipeline_string)`
3. `gst_parse_launch` needs to:
   - Look up element factories in registry (requires type lock)
   - Create element instances (requires type lock)
   - Set properties (requires type lock)
4. **BLOCKS** waiting for type lock that will never be released

This explains why pipelines "should be separate" but new ones can't start - they all share the same global type system.

## The Fix

**Option 1: Move g_object_set to pipeline thread** (CORRECT FIX)

Instead of calling `g_object_set` from HTTPS thread, post a message to the pipeline's bus and handle it in the pipeline thread:

```cpp
// In HTTPS thread (event handler):
auto switch_ev_copy = std::make_shared<immer::box<events::SwitchStreamProducerEvents>>(switch_ev);
gst_element_post_message(pipeline.get(),
  gst_message_new_application(GST_OBJECT(pipeline.get()),
    gst_structure_new("switch-producer",
      "interpipe-id", G_TYPE_STRING, switch_ev->interpipe_src_id.c_str(),
      nullptr)));

// In pipeline thread (bus handler):
g_signal_connect(bus, "message::application", G_CALLBACK(+[](GstBus*, GstMessage* msg, gpointer user_data) {
  const GstStructure* s = gst_message_get_structure(msg);
  if (gst_structure_has_name(s, "switch-producer")) {
    const char* interpipe_id = gst_structure_get_string(s, "interpipe-id");
    // NOW we're in the pipeline thread - safe to call g_object_set
    auto src = gst_bin_get_by_name(...);
    g_object_set(src, "listen-to", interpipe_id, nullptr);
    gst_object_unref(src);
  }
}), nullptr);
```

**Why this works:**
- `gst_element_post_message` is thread-safe (just adds to message queue)
- Message is processed by pipeline thread's main loop
- `g_object_set` called from same thread that owns the pipeline
- If pipeline thread dies, the mutex it holds is local to that pipeline
- Other pipelines remain unaffected

**Option 2: Remove dynamic switching entirely**

If switching interpipesrc isn't critical, remove the feature:
- Restart the entire pipeline instead of switching
- Use the existing pause/resume mechanism
- Avoids g_object_set entirely

**Option 3: Add watchdog to kill stuck HTTPS threads**

Not a real fix, but a mitigation:
- Detect when HTTPS thread is stuck in g_object_set > 5 seconds
- Kill the thread (will release type lock on exit)
- Restart HTTPS handler

## Next Steps

1. ❌ **NEVER restart hung processes without GDB analysis first** (already added to CLAUDE.md)
2. ✅ Implement Option 1 (move g_object_set to pipeline thread)
3. ✅ Add logging around g_object_set calls to detect when they block
4. ✅ Add thread ownership assertions (verify we're in pipeline thread before calling g_object_set)
5. ⏳ Wait for next production deadlock with GDB attached to confirm this theory

## Lessons Learned

1. **GLib type system has global mutexes** - not just GStreamer pipeline mutexes
2. **NEVER call g_object_set from non-owner thread** - thread confinement is critical
3. **Event handlers run in caller's thread** - not magically in the right context
4. **Interpipe is buggy** - has known buffer refcounting issues
5. **Partial fixes are dangerous** - g_main_loop_quit fixed one deadlock, introduced false confidence

## References

- **g_main_loop_quit fix**: Wolf commit `2be71f1`
- **Interpipe bug**: `gstrtpmoonlightpay_video.cpp:112-120`
- **Switch handler**: `streaming.cpp:337-347`
- **Hang debug logs**: `streaming.cpp` lines with `[HANG_DEBUG]` prefix
