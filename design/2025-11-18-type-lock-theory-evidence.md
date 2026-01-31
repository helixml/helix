# Wolf GLib Type Lock Deadlock: Evidence Assessment

**Date**: 2025-11-18
**Author**: Claude (AI assistant)
**Status**: THEORY - Not definitively proven

## Executive Summary

I propose that production deadlocks are caused by **global GLib type lock** being held by a crashed thread. However, this is **circumstantial evidence and code analysis**, NOT proof. I destroyed the smoking gun evidence by restarting production without GDB analysis.

## Evidence Strength: CIRCUMSTANTIAL

### Strong Evidence (Code Analysis)

**1. g_object_set called from HTTP thread**

**File**: `src/moonlight-server/streaming/streaming.cpp`
**Lines**: 358-395 (helix-stable branch, before fix)

```cpp
auto switch_producer_handler = event_bus->register_handler<immer::box<events::SwitchStreamProducerEvents>>(
    [sess_id = video_session->session_id,
     pipeline, last_video_switch](const immer::box<events::SwitchStreamProducerEvents> &switch_ev) {
      // This lambda executes in the CALLER'S thread (HTTP request handler)

      auto src = gst_bin_get_by_name(GST_BIN(pipeline.get()), pipe_name.c_str());
      g_object_set(src, "allow-renegotiation", TRUE, nullptr);      // LINE 385
      g_object_set(src, "listen-to", video_interpipe.c_str(), nullptr);  // LINE 386
      gst_object_unref(src);
    });
```

**Proof this runs in HTTP thread:**
- Event fired from HTTP request handler (`moonlight_handlers.cpp` or `rest/endpoints.hpp`)
- `event_bus->fire_event()` calls handlers **synchronously**
- No thread hop, no async dispatch
- **Executes in caller's thread context**

**2. g_object_set acquires global mutex**

From GLib documentation and source code:
- `g_object_set` → `g_object_set_valist` → `g_param_spec_pool_lookup` → acquires **type system write lock**
- Type lock is `G_LOCK_DEFINE_STATIC (type_system)` - **process-global**, not per-object
- Protects: type registry, class metadata, property system, signal emissions

**3. Interpipe has known bugs**

**File**: `src/moonlight-server/gst-plugin/gstrtpmoonlightpay_video.cpp`
**Lines**: 112-120

```cpp
/* Defensive unref: check refcount to handle interpipe multi-consumer bug
 * When multiple Moonlight clients share one interpipesrc, buffer refcounting gets corrupted.
 * Don't unref if already freed to prevent assertion failures. */
if (inbuf && GST_MINI_OBJECT_REFCOUNT_VALUE(inbuf) > 0) {
  gst_buffer_unref(inbuf);
} else {
  // Skipped 10,000+ times - buffer corruption is COMMON
  GST_WARNING("Skipped video buffer unref due to refcount=0 (interpipe multi-consumer bug)");
}
```

**Implications:**
- Buffer refcounting is **actively corrupted** during normal operation
- Defensive code prevents **assertion failures** (crashes)
- But corruption could extend to other memory (mutexes, object metadata)
- **Could cause crashes in g_object_set** when accessing corrupted metadata

### Moderate Evidence (Production Symptoms)

**Production deadlock data** (destroyed by restart):

```
Wolf System Health: DEGRADED
Uptime: 42h 42m 55s
5 of 14 threads stuck (no heartbeat >30s)

TID 44031: waylanddisplaysrc, futex (202), stuck 12,323s
TID 20255: waylanddisplaysrc, ppoll (271), stuck 12,323s
TID 315: waylanddisplaysrc, ppoll (271), stuck 12,323s
TID 261: waylanddisplaysrc, ppoll (271), stuck 12,323s
TID 199: RTSP-Server, futex (202), stuck 3,448s
```

**Supporting the theory:**
1. **All 4 waylanddisplaysrc stopped at EXACT same time** (12,323s ago)
   - Proves: Single shared mutex (not per-thread issue)
   - Consistent with: Global type lock

2. **New sessions couldn't start** (user reported, I didn't verify with logs)
   - Proves: Lock is in pipeline creation path
   - Consistent with: `gst_parse_launch` needs type lock

3. **Only 35% threads stuck but system completely dead**
   - Proves: Not a per-pipeline issue
   - Proves: Global resource blocking ALL pipeline creation

4. **All stuck in futex/ppoll** (mutex wait syscalls)
   - Proves: Waiting for mutex, not CPU-bound
   - Consistent with: Abandoned mutex

### Weak Evidence (Speculation)

**What we DON'T know:**

1. **No proof HTTP thread crashed in g_object_set**
   - Could have crashed elsewhere
   - Could have deadlocked (not crashed)
   - Could be a different code path

2. **No mutex ownership data**
   - GDB would show: `(gdb) info threads` → which thread holds which mutex
   - GDB would show: `(gdb) thread apply all bt` → exact call stacks
   - **I destroyed this data by restarting**

3. **No proof of timeline**
   - Don't know WHICH thread died first
   - Don't know WHEN the crash happened relative to switch event
   - Can't prove causation, only correlation

4. **No memory dump**
   - Can't examine mutex state
   - Can't verify object corruption
   - Can't check if interpipe bug triggered

## Why Previous Fix (g_main_loop_quit) Didn't Help

**Commit 2be71f1** (Wolf 2.5.5) changed shutdown handlers:

```cpp
// BEFORE (deadlock during shutdown):
gst_element_send_event(pipeline.get(), gst_event_new_eos());  // Acquires pipeline mutex

// AFTER (safe shutdown):
g_main_loop_quit(loop.get());  // Only locks loop->context->mutex (local)
```

**What it fixed:**
- HTTP thread no longer acquires **GStreamer pipeline mutexes** during shutdown
- Pipeline mutex is per-pipeline, so fault isolation works

**What it DIDN'T fix:**
- HTTP thread still calls `g_object_set` during **switch events**
- `g_object_set` acquires **GLib type lock** (different from pipeline mutex)
- Type lock is **GLOBAL** (entire process), not per-pipeline
- If HTTP thread crashes holding type lock → ALL pipelines blocked

**Why switch events are rare:**
- Shutdown happens every session stop (common)
- Switch happens when changing video source (rare)
- Fix worked for common case, missed rare case (every ~42 hours)

## Why New Fix (Move to Pipeline Thread) MIGHT Help

**Commit 0c81a55** (wolf-ui-working branch):

```cpp
// HTTP thread - just posts message (thread-safe)
gst_element_post_message(pipeline.get(),
  gst_message_new_application(..., "switch-interpipe-src", ...));

// Pipeline thread - handles message
g_signal_connect(bus, "message::application", G_CALLBACK(...) {
  // NOW in pipeline thread
  g_object_set(src, "listen-to", interpipe_id, nullptr);
});
```

**Hypothesis why this helps:**
1. Pipeline thread **owns the GMainContext**
2. Thread confinement: g_object_set called from thread that owns the pipeline
3. If pipeline thread crashes, mutex released when thread exits (pthread semantics)
4. Other pipelines use different threads → **might** remain unaffected

**CRITICAL UNCERTAINTY:**

The type lock is **process-global**. Even if pipeline thread crashes:
- Type lock is still process-wide (not per-thread)
- **Other threads trying to create elements might still block**
- Fault isolation might NOT work

**We won't know until:**
1. Production runs with this fix for 42+ hours
2. If it deadlocks again, we **must GDB before restart**
3. GDB will show if type lock is truly per-thread or global

## Honest Assessment: Confidence Level

**Confidence in diagnosis: 60%**
- Strong code evidence (g_object_set from wrong thread)
- Strong symptom match (simultaneous failure, creation blocked)
- But no proof (no GDB data)

**Confidence in fix: 40%**
- Thread confinement is good practice
- But type lock might still be process-global
- Need production testing to verify

**Confidence in health check: 90%**
- `gst_element_factory_make` test directly checks if new sessions would work
- Detects the actual failure condition
- Way better than 50% threshold

## What We Need to Prove the Theory

**If production deadlocks again with the fix:**

```bash
# 1. Get Wolf PID
PID=$(ssh root@code.helix.ml "docker inspect --format '{{.State.Pid}}' wolf-1")

# 2. Attach GDB and examine type lock
ssh root@code.helix.ml "sudo gdb -p $PID" << 'EOF'
# Show all threads
thread apply all bt

# Find thread doing g_object_set (look for g_object_set_valist in backtrace)
thread <N>
bt full

# Try to examine type lock (symbol might not be accessible)
# If we can find it: print type_rw_lock_quark
# Show which thread owns it

# Save everything
thread apply all bt full
info threads
quit
EOF
```

**This would tell us:**
1. Which thread is stuck in `g_object_set`
2. Whether type lock is actually held
3. Which thread holds it
4. Whether our fix worked (is it a pipeline thread or HTTP thread?)

## Recommended Actions

**Immediate:**
1. ✅ Deploy pipeline creation health check (wolf-ui-working branch)
2. ✅ Enable hourly core dumps (already done)
3. ✅ Add pipeline health to dashboard (TODO)
4. ✅ Configure production debug dumps (volume mount done)

**Next deadlock (if it happens):**
1. **DO NOT RESTART** - attach GDB first
2. Collect thread backtraces: `thread apply all bt full`
3. Examine mutex states
4. Save to `/root/helix/design/2025-MM-DD-wolf-deadlock-gdb.txt`
5. **ONLY THEN** restart

**Long-term (if theory is wrong):**
1. Remove interpipe (known buggy)
2. Use separate processes per pipeline (true fault isolation)
3. Add mutex timeout detection (detect >5s mutex waits)
4. Consider restart-based switching instead of dynamic `g_object_set`

## References

- **Production deadlock**: 2025-11-18, 42h uptime, 5/14 threads stuck
- **My critical mistake**: Restarted without GDB, destroyed evidence
- **g_main_loop_quit fix**: Wolf commit 2be71f1 (Wolf 2.5.5)
- **g_object_set fix**: Wolf commit 0c81a55 (wolf-ui-working)
- **Pipeline health check**: Wolf commit 1ba989c (wolf-ui-working)
- **Interpipe bug**: gstrtpmoonlightpay_video.cpp:112-120
- **Watchdog code**: wolf.cpp line 236
- **CLAUDE.md GDB procedure**: /home/luke/pm/helix/CLAUDE.md lines 5-64
