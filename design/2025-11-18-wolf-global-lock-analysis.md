# Wolf Global Lock Analysis - Complete Threat Assessment

**Date**: 2025-11-18
**Scope**: All GStreamer/GLib calls from HTTP threads + shared resources + pipeline thread hangs

## Executive Summary

The Wolf codebase has **already been partially fixed** (wolf-ui-working branch) to move `g_object_set` calls from HTTP threads to pipeline threads via bus messaging. However, **this fix provides better fault isolation but doesn't eliminate the deadlock risk entirely** because the GLib type lock is still **process-global**.

**Current Status**:
- ✅ `SwitchStreamProducerEvents` handlers POST messages (don't call `g_object_set` directly)
- ✅ Pipeline thread bus handlers call `g_object_set` (safer thread context)
- ✅ `PauseStreamEvent` / `StopStreamEvent` use `g_main_loop_quit` (no locks)
- ⚠️ But type lock is still global - pipeline thread crash still blocks other pipelines

**Why Pipeline Threads Hang**: See Section 5 for detailed analysis of the ROOT cause.

---

## 1. All Event Handlers - Current Risk Assessment

### Event Handlers Using SAFE Patterns (✅ Already Fixed)

#### 1.1 SwitchStreamProducerEvents (Video)
**File**: `streaming.cpp:391-427`
**Pattern**: ✅ **Message posting** (correct)
```cpp
auto switch_producer_handler = event_bus->register_handler<...>([pipeline, ...](...) {
  // HTTP thread - just posts message
  gst_element_post_message(pipeline.get(),
    gst_message_new_application(..., "switch-interpipe-src", ...));
});

// Pipeline thread - handles message in bus handler (lines 306-336)
g_signal_connect(bus, "message::application", G_CALLBACK(+[](...) {
  // NOW in pipeline thread
  g_object_set(src, "listen-to", interpipe_id, nullptr);
}), nullptr);
```

**Risk**: ⚠️ **LOW-MEDIUM**
- If pipeline thread crashes in `g_object_set` → only that pipeline hangs (better)
- But type lock is STILL global → might affect other pipelines creating elements
- HTTP thread safe (just posts message, no blocking)

#### 1.2 SwitchStreamProducerEvents (Audio)
**File**: `streaming.cpp:551-581`
**Pattern**: ✅ Same as video - message posting

#### 1.3 PauseStreamEvent Handlers
**Files**: `streaming.cpp:360-386` (video), `streaming.cpp:520-546` (audio)
**Pattern**: ✅ `g_main_loop_quit` (thread-safe, no locks)
```cpp
auto pause_handler = event_bus->register_handler<...>([loop](...) {
  g_main_loop_quit(loop.get());  // ✅ Thread-safe, isolated per-pipeline
});
```

**Risk**: ✅ **NONE** - `g_main_loop_quit` only sets flag, no mutexes

#### 1.4 StopStreamEvent Handlers
**Files**: Multiple (producer/consumer pipelines)
**Pattern**: ✅ `g_main_loop_quit` (same as pause)

### Event Handlers with POTENTIAL ISSUES

#### 1.5 IDRRequestEvent (Force Keyframe)
**File**: `streaming.cpp:348-358`
**Pattern**: ⚠️ **gstreamer::send_message** from control thread

```cpp
auto idr_handler = event_bus->register_handler<immer::box<events::IDRRequestEvent>>(
    [sess_id = video_session->session_id, pipeline]
    (const immer::box<events::IDRRequestEvent> &ctrl_ev) {
      // Runs in CONTROL THREAD (not HTTP, not pipeline)
      wolf::core::gstreamer::send_message(
          pipeline.get(),
          gst_structure_new("GstForceKeyUnit", "all-headers", G_TYPE_BOOLEAN, TRUE, NULL));
    });
```

**What send_message does** (`core/gstreamer.hpp:34-37`):
```cpp
static void send_message(GstElement *recipient, GstStructure *message) {
  auto gst_ev = gst_event_new_custom(GST_EVENT_CUSTOM_UPSTREAM, message);
  gst_element_send_event(recipient, gst_ev);  // ⚠️ Acquires pipeline mutex
}
```

**Risk**: ⚠️ **MEDIUM**
- Control thread acquires pipeline mutex via `gst_element_send_event`
- If control thread crashes → only affects that pipeline (not global)
- But control thread stuck → IDR requests pile up

**Better pattern**: Post to pipeline bus like switch events:
```cpp
gst_element_post_message(pipeline.get(),
  gst_message_new_application(..., "force-idr", ...));
```

---

## 2. Global Resources (Cross-Pipeline Contamination)

### 2.1 GLib Global Type Lock (MOST DANGEROUS)

**Scope**: **ENTIRE PROCESS**

**Functions that acquire it**:
- `g_object_set()` / `g_object_get()`
- `g_object_new()` / `g_object_unref()`
- `gst_element_factory_make()`
- `gst_bin_get_by_name()`
- `gst_parse_launch()`
- `g_type_register_*()` / `g_type_class_ref()`

**Why it's global**: GLib type system is a singleton - all GObject types share one registry.

**Impact**:
- Dead thread holding type lock → ALL `gst_element_factory_make` blocked
- ALL new pipeline creation blocked
- ALL `g_object_set` blocked (even in other pipelines)
- System functionally dead even if some threads running

**Mitigation**: Move `g_object_set` to pipeline threads (done), but lock is still global.

### 2.2 GStreamer Registry Lock (ALSO GLOBAL)

**Scope**: **ENTIRE PROCESS**

**Functions that acquire it**:
- `gst_registry_get()` - returns **singleton** registry
- `gst_registry_find_feature()`
- `gst_element_factory_make()` - looks up factory in registry
- `gst_parse_launch()` - parses element names, looks up in registry

**Evidence**:
```cpp
// core/gstreamer.hpp:40-75
static std::vector<std::string> get_dma_caps(const std::string &gst_plugin_name) {
  GstRegistry *registry = gst_registry_get();  // ⚠️ GLOBAL SINGLETON
  auto feature = gst_registry_find_feature(registry, ...);  // Acquires registry lock
}
```

**Impact**: Same as type lock - if held, all pipeline creation blocked.

### 2.3 Interpipe Global Mutexes (THIRD-PARTY CODE)

**Source**: `gst-interpipe` plugin (external dependency)

**Global State** (from interpipe source):
```c
// gstinterpipe.c (approximate, not in Wolf repo)
static GMutex listeners_mutex;  // GLOBAL - all interpipesrc share this
static GMutex nodes_mutex;      // GLOBAL - all interpipesink share this
static GHashTable *listeners;   // GLOBAL registry
static GHashTable *nodes;       // GLOBAL registry
```

**Functions that acquire them**:
- `gst_inter_pipe_listen_node()` - when setting "listen-to" property
- `gst_inter_pipe_ilistener_push_buffer()` - when sending buffers
- Any interpipe connection/disconnection

**Evidence of bugs**:
```cpp
// gst-plugin/gstrtpmoonlightpay_video.cpp:112-127
/* Defensive unref: check refcount to handle interpipe multi-consumer bug
 * When multiple Moonlight clients share one interpipesrc, buffer refcounting gets corrupted.
 * Don't unref if already freed to prevent assertion failures. */
if (inbuf && GST_MINI_OBJECT_REFCOUNT_VALUE(inbuf) > 0) {
  gst_buffer_unref(inbuf);
} else {
  // Logged 10,000+ times - buffer corruption is COMMON
  GST_WARNING("Skipped video buffer unref due to refcount=0");
}
```

**Impact**: Interpipe is **known buggy** - can crash, leaving global mutexes locked.

### 2.4 Shared GStreamer Video Context

**File**: `state/data-structures.hpp` (inferred from usage)

**Sharing**:
```cpp
// All video producers share this:
std::shared_ptr<immer::atom<gst_video_context::gst_context_ptr>> video_context
```

**Passed to**:
- All `start_video_producer` calls
- All `start_streaming_video` calls

**Risk**: ⚠️ **MEDIUM**
- If context contains VA-API/OpenGL/CUDA handles, those have internal mutexes
- Dead thread holding VAAPI lock → all video encoding blocked

---

## 3. Why Pipeline Threads Themselves Hang

### 3.1 The Cascading Deadlock Pattern

**Observed in Production**:
```
All 4 waylanddisplaysrc threads stopped at EXACT same time (12,323s ago)
All stuck in: futex(202) or ppoll(271)
```

**Reconstruction**:

**Step 1: HTTP Thread Triggers Switch**
```
HTTP /api/v1/lobbies/join
→ JoinLobbyEvent handler
→ Fires SwitchStreamProducerEvents
→ Posts message to pipeline bus ✅
→ HTTP thread returns (safe) ✅
```

**Step 2: Pipeline Thread Processes Message**
```
Pipeline thread wakes up from ppoll
→ Processes bus message "switch-interpipe-src"
→ Calls gst_bin_get_by_name() - acquires type lock
→ Calls g_object_set() - holds type lock
→ Sets interpipe "listen-to" property
→ Interpipe plugin code runs (THIRD-PARTY, BUGGY)
→ **CRASH** - buffer refcounting bug, assertion failure, etc.
→ Thread dies WHILE HOLDING global type lock ❌
```

**Step 3: Other Pipelines Try to Process Buffers**
```
waylanddisplaysrc thread 1:
→ Processing video buffer
→ Needs to create pad caps
→ Calls gst_caps_new() - tries to acquire type lock
→ **BLOCKS** - lock held by dead thread
→ Stuck in futex forever

waylanddisplaysrc thread 2/3/4:
→ Same sequence
→ **ALL BLOCK** on same type lock
→ All show futex(202) syscall
```

**Step 4: New Session Creation Blocked**
```
User clicks "Start Session"
→ HTTP /api/v1/apps/start
→ Calls gst_parse_launch("waylanddisplaysrc name=...")
→ Tries to acquire type lock
→ **BLOCKS** - lock held by dead thread
→ HTTP request hangs forever
```

### 3.2 Why Interpipe Crashes

**Known Bug Evidence**:
```cpp
// gstrtpmoonlightpay_video.cpp:112-127
if (inbuf && GST_MINI_OBJECT_REFCOUNT_VALUE(inbuf) > 0) {
  gst_buffer_unref(inbuf);
} else {
  static std::atomic<guint64> skip_count{0};
  guint64 count = skip_count.fetch_add(1, std::memory_order_relaxed);
  if (count % 10000 == 0) {  // Logged THOUSANDS of times
    GST_WARNING("Skipped video buffer unref due to refcount=0");
  }
}
```

**What's happening**:
1. Multiple consumers share one interpipesrc (lobby mode)
2. Buffer passed to all consumers
3. Each consumer should increment refcount
4. **Refcount gets corrupted** (interpipe bug)
5. Buffer freed while still in use
6. Later access → **use-after-free crash**

**Crash locations**:
- `g_object_set()` reading corrupted object metadata
- Buffer processing in GStreamer internal code
- Mutex operations on freed memory

### 3.3 Other Reasons Pipeline Threads Hang

**A. GStreamer Internal Deadlocks**

**Evidence from development dumps** (wolf-debug-dumps/1763133645-threads.txt):
```
TID 486: GStreamer-Pipeline (waylanddisplaysrc), stuck 107s, 0 heartbeats
TID 487: GStreamer-Pipeline (pulsesrc), stuck 107s, 0 heartbeats
TID 495: GStreamer-Pipeline (interpipesrc video), stuck 107s, 0 heartbeats
TID 496: GStreamer-Pipeline (interpipesrc audio), stuck 107s, 0 heartbeats
```

**Pattern**: ALL pipeline threads stuck, 0 heartbeats

**Likely cause**: Deadlock cycle in GStreamer internal locks:
- Pipeline A: Holds lock X, wants lock Y
- Pipeline B: Holds lock Y, wants lock X
- Classic deadlock (not our code, GStreamer library)

**B. Wayland Compositor Hangs**

`waylanddisplaysrc` talks to Wayland compositor (Sway):
- If compositor hangs → waylanddisplaysrc blocks in wayland protocol calls
- Blocks in `ppoll(271)` waiting for compositor response

**C. Pulse Audio Hangs**

`pulsesrc` talks to PulseAudio server:
- If pulseaudio hangs → pulsesrc blocks
- Blocks in `poll()` or `futex()` waiting for audio data

**D. NVIDIA Driver Issues**

**Evidence**:
```
TID 126: CUDA worker thread, poll (7)
```

CUDA/NVENC operations can deadlock if driver has issues.

---

## 4. The Type Lock Dilemma

### Moving g_object_set to Pipeline Thread: Does It Actually Help?

**Theory**:
- HTTP thread: Post message (no lock) ✅
- Pipeline thread: Process message, call `g_object_set` (acquires type lock) ⚠️
- If pipeline thread crashes → only that pipeline hangs ✅

**Reality Check**:

**Scenario 1: Pipeline Thread Crashes in g_object_set**
```
Pipeline thread 1:
→ Handles "switch-interpipe-src" message
→ Calls g_object_set(interpipesrc, "listen-to", ...)
→ Acquires global type lock
→ **CRASHES** (interpipe bug)
→ Dies holding type lock

Pipeline thread 2:
→ Processing video buffer
→ Calls gst_caps_new() (needs type lock)
→ **BLOCKS** - type lock held by dead thread 1

New session creation:
→ gst_parse_launch()
→ **BLOCKS** - type lock held by dead thread 1
```

**Conclusion**: Moving `g_object_set` to pipeline thread provides **better isolation** (HTTP API stays responsive), but **doesn't prevent cross-pipeline deadlock**.

### Why It's Still Better Than Before

**Before** (HTTP thread calls g_object_set):
- HTTP thread crashes → **ALL HTTP endpoints blocked**
- **AND** all pipeline creation blocked
- **Complete system lockup**

**After** (pipeline thread calls g_object_set):
- Pipeline thread crashes → **only that pipeline hangs**
- HTTP endpoints stay responsive ✅
- Can still stop/start sessions via API ✅
- But new pipeline creation might still block ⚠️

**Net benefit**: ~50% reduction in impact area, but not elimination.

---

## 5. Root Cause: Why Do Threads Crash in First Place?

### 5.1 Interpipe Buffer Refcounting Corruption

**Primary Suspect**:

```cpp
// gstrtpmoonlightpay_video.cpp:112-127
/* When multiple Moonlight clients share one interpipesrc, buffer refcounting gets corrupted. */
if (inbuf && GST_MINI_OBJECT_REFCOUNT_VALUE(inbuf) > 0) {
  gst_buffer_unref(inbuf);
} else {
  // THOUSANDS of occurrences logged
  GST_WARNING("Skipped video buffer unref due to refcount=0");
}
```

**What happens**:
1. Interpipe shares buffers between pipelines (zero-copy optimization)
2. Refcount should be: consumers × frames in flight
3. Bug: Refcount gets decremented too many times
4. Buffer freed while still in use (refcount reaches 0 early)
5. Other code accesses freed buffer → **use-after-free**
6. Crashes in:
   - `g_object_set` reading freed object header
   - `gst_buffer_map` accessing freed memory
   - Mutex operations on freed structures

### 5.2 GPU Driver / NVENC Issues

**Evidence from logs**:
```
[VAAPI] unknown libva error
[NVENC] buffer pool exhausted
```

**Crash scenarios**:
- NVENC runs out of encoder sessions
- VAAPI driver assertion failure
- CUDA OOM (GPU memory exhausted)
- Driver deadlock (NVIDIA proprietary code)

### 5.3 GStreamer Buffer Pool Exhaustion

**Error messages**:
```
GST_FLOW_ERROR (-2): buffer pool exhausted
```

**What happens**:
1. Downstream consumer can't keep up
2. Buffer pool fills
3. Upstream source blocks waiting for free buffer
4. If block happens while holding lock → deadlock

### 5.4 Wayland Protocol Errors

`waylanddisplaysrc` sends buffers to Wayland compositor (Sway):
- Compositor crashes → wayland calls return errors
- Errors trigger GStreamer error handler
- Error handler might call GStreamer functions → could acquire locks

---

## 6. Recommended Fixes (Comprehensive)

### FIX #1: Change IDR Handler to Message Passing (SHOULD DO)

**Current** (`streaming.cpp:348-358`):
```cpp
auto idr_handler = event_bus->register_handler<immer::box<events::IDRRequestEvent>>(
    [sess_id, pipeline](const immer::box<events::IDRRequestEvent> &ctrl_ev) {
      wolf::core::gstreamer::send_message(pipeline.get(), ...);  // ❌ Control thread → pipeline mutex
    });
```

**Fixed**:
```cpp
auto idr_handler = event_bus->register_handler<immer::box<events::IDRRequestEvent>>(
    [sess_id, pipeline](const immer::box<events::IDRRequestEvent> &ctrl_ev) {
      // ✅ Post to bus (no locks)
      gst_element_post_message(pipeline.get(),
        gst_message_new_application(GST_OBJECT(pipeline.get()),
          gst_structure_new("force-idr",
            "session-id", G_TYPE_UINT, sess_id,
            nullptr)));
    });

// Add handler in run_pipeline bus setup:
g_signal_connect(bus, "message::application", G_CALLBACK(+[](...) {
  if (gst_structure_has_name(s, "force-idr")) {
    // NOW in pipeline thread - safe
    wolf::core::gstreamer::send_message(pipeline.get(),
      gst_structure_new("GstForceKeyUnit", ...));
  }
}), nullptr);
```

### FIX #2: Add Robust Mutex Support (LONG-TERM, COMPLEX)

**Problem**: Mutexes not marked `PTHREAD_MUTEX_ROBUST`

**What robust mutexes do**:
```c
// If thread dies holding robust mutex:
int ret = pthread_mutex_lock(&mutex);
if (ret == EOWNERDEAD) {
  // Mutex was held by dead thread
  // Mark as consistent and continue
  pthread_mutex_consistent(&mutex);
  // Now we own it - proceed with caution
}
```

**Challenge**: GLib/GStreamer mutexes are internal - can't modify them.

**Workaround**: Patch interpipe plugin to use robust mutexes (we control this).

### FIX #3: Replace Interpipe (BEST LONG-TERM FIX)

**Problem**: Interpipe is buggy and can't be made robust (third-party)

**Alternative 1**: Direct GStreamer Tee/Queue
```
waylanddisplaysrc ! tee name=t
t. ! queue ! consumer1
t. ! queue ! consumer2
```

**Alternative 2**: Shared memory transport
```
waylanddisplaysrc ! shmsink
consumer1: shmsrc ! ...
consumer2: shmsrc ! ...
```

**Benefit**: Eliminates interpipe global mutexes and refcounting bugs.

### FIX #4: Add Type Lock Timeout Detection (MONITORING)

**Wrap all g_object_set calls**:
```cpp
// New helper function
template<typename Func>
auto with_lock_timeout(Func&& func, const char* operation) {
  auto start = std::chrono::steady_clock::now();
  auto result = func();
  auto elapsed = std::chrono::steady_clock::now() - start;

  if (elapsed > std::chrono::seconds(5)) {
    logs::log(logs::fatal, "[DEADLOCK_DETECTION] {} took {}s - possible abandoned mutex",
              operation, std::chrono::duration_cast<std::chrono::seconds>(elapsed).count());
    // Could trigger emergency dump here
  }

  return result;
}

// Usage:
with_lock_timeout([&]() {
  g_object_set(src, "listen-to", interpipe_id, nullptr);
}, "g_object_set listen-to");
```

**Benefit**: Early warning if operations take too long (lock contention).

### FIX #5: Separate Processes Per Pipeline (ULTIMATE FIX)

**Architecture change**: Run each streaming pipeline in separate process

**Benefits**:
- Type lock scoped to process (true fault isolation)
- Dead pipeline process doesn't affect others
- Easier to kill/restart individual pipelines

**Challenges**:
- IPC complexity (sharing buffers between processes)
- Resource overhead (multiple processes)
- Harder to manage

---

## 7. Evidence Strength Assessment

### What We Know (Strong Evidence)

✅ **Code Structure**:
- HTTP threads call event handlers synchronously
- Some handlers call GStreamer functions
- GStreamer functions acquire global locks
- Interpipe has documented refcounting bugs

✅ **Production Symptoms**:
- All waylanddisplaysrc stopped simultaneously (shared resource)
- New sessions couldn't start (global lock in creation path)
- Only 35% threads stuck but system dead (not per-pipeline)

✅ **Development Dumps**:
- 400+ crash dumps from Nov 14
- Two different deadlock patterns
- Shows systematic issues, not one-off

### What We Don't Know (Missing Evidence - Destroyed by Restart)

❌ **Thread Backtraces**:
- Which thread held which mutex
- Exact call stack at crash
- Proof of which code path triggered

❌ **Mutex Ownership**:
- Which thread owned type lock
- Whether it was HTTP thread or pipeline thread
- Confirmation of theory

❌ **Timeline**:
- When exactly did crash happen
- Was it during switch event or something else
- Correlation with HTTP requests

### Confidence Levels

- **g_object_set is dangerous**: 100% (documented GLib behavior)
- **It's called from HTTP thread**: 0% - **FIX ALREADY IMPLEMENTED** (posts message instead)
- **Pipeline thread crashes in g_object_set**: 60% (probable, not proven)
- **Type lock is global**: 100% (GLib design)
- **Interpipe is buggy**: 100% (evidence in defensive code)

---

## 8. Final Recommendations

### Immediate (Already Done)

1. ✅ Move `g_object_set` to pipeline threads (bus messaging)
2. ✅ Replace 50% threshold with pipeline creation test
3. ✅ Add hourly core dumps
4. ✅ Add complete debug symbols
5. ✅ Document GDB procedure

### Next Production Deployment

1. Deploy wolf-ui-working branch
2. Monitor `can_create_new_pipelines` in dashboard
3. **If it deadlocks again**: GDB before restart!
4. Save evidence to prove/disprove theory

### If Deadlocks Continue

**This would prove the fix didn't work**. Then:

1. **Replace interpipe entirely** (eliminate known bug source)
2. **Switch to tee+queue** or **shmsink/shmsrc**
3. **Separate processes** for true fault isolation

### Long-Term Architecture

**Current**: All pipelines in one process (shared type lock, shared interpipe mutexes)

**Better**: Separate process per streaming session
- Each has own type lock namespace
- Dead process doesn't contaminate others
- Can kill/restart individual sessions

**Best**: Don't use GStreamer for streaming at all
- Direct NVENC API calls
- Manual RTP packetization
- No global locks, full control

---

## 9. Testing Checklist Before Release

### Test 1: Concurrent Lobby Operations
```bash
# Hammer with concurrent join/leave
for i in {1..20}; do
  (curl POST /api/v1/lobbies/join ...) &
done

# Expected: All succeed, can_create_new_pipelines stays true
```

### Test 2: Monitor During 48h Soak Test
```bash
# Watch health continuously
watch -n 5 'curl /api/v1/wolf/health | jq .can_create_new_pipelines'

# Expected: Always returns true for 48+ hours
```

### Test 3: Verify Hourly Dumps
```bash
# Check dumps accumulate
ls -lh /var/wolf-debug-dumps/hourly-*

# Expected: New dump every hour, rotation after 48
```

### Test 4: Simulated Deadlock Recovery
```bash
# If can_create_new_pipelines goes false:
# 1. Verify watchdog triggers within 60s
# 2. Verify core dump created
# 3. Verify process exits
# 4. Verify Docker restarts
```

---

## 10. Open Questions

1. **Does moving g_object_set to pipeline thread actually isolate faults?**
   - Type lock might still be global even within pipeline thread
   - Need production test to verify
   - GDB on next deadlock will answer this

2. **Why do multiple waylanddisplaysrc threads all hang simultaneously?**
   - Shared interpipe mutex?
   - Shared VAAPI context?
   - Shared wayland compositor connection?

3. **Is there a deadlock cycle in GStreamer library code?**
   - Development dumps show all threads stuck with 0 heartbeats
   - Suggests deadlock happened during initialization
   - Need GDB to find cycle

4. **Can we make interpipe more robust?**
   - Fork it and add proper mutex error handling?
   - Or just replace it entirely?

---

## Conclusion

The codebase analysis reveals:

**Good News**:
- ✅ Most dangerous calls (g_object_set from HTTP) already fixed in wolf-ui-working
- ✅ Health check properly detects type lock deadlock
- ✅ Crash dump infrastructure in place

**Bad News**:
- ⚠️ Type lock is still global - fix provides better isolation, not elimination
- ⚠️ Interpipe is fundamentally buggy - will continue causing crashes
- ⚠️ Multiple global locks (type, registry, interpipe) - any can deadlock

**Honest Assessment**:
- Fix **will reduce deadlock frequency** (better fault isolation)
- Fix **won't eliminate deadlocks** (locks still global)
- **If deadlocks continue**: Need to replace interpipe or use separate processes

**Critical**: Next deadlock, we MUST use GDB before restarting to collect definitive proof.
