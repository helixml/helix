# Wolf Deadlock Investigation - Final Summary

**Date**: 2025-11-18
**Production Issue**: Wolf deadlocks every ~42 hours, blocking ALL streaming

## What We Fixed Today

### 1. Critical Debugging Rules (CLAUDE.md)

Added **mandatory GDB procedure** before restarting hung processes:
- Get process ID
- Attach GDB and collect thread backtraces
- Examine mutex states and deadlock cycles
- Check system state and save everything
- **ONLY THEN restart**

**Why**: Restarting destroys irreplaceable debugging information. I made this mistake today by restarting production Wolf without GDB - destroyed all evidence.

### 2. Root Cause Analysis (60% Confidence)

**Theory**: HTTP thread calls `g_object_set` during `SwitchStreamProducerEvent`, acquires global GLib type lock, crashes while holding it → all pipeline creation blocks.

**Supporting evidence:**
- ✅ Code shows `g_object_set` from HTTP event handler (`streaming.cpp:385-386`)
- ✅ All 4 waylanddisplaysrc stopped simultaneously (shared resource)
- ✅ New sessions couldn't start (global lock in creation path)
- ✅ Only 35% threads stuck but system dead (not per-pipeline issue)
- ✅ Interpipe has known buffer corruption bugs

**Missing evidence** (destroyed by restart):
- ❌ Thread backtraces showing exact deadlock location
- ❌ Mutex ownership data
- ❌ Proof HTTP thread crashed in `g_object_set`

### 3. Code Fixes (Wolf wolf-ui-working branch)

**A. Move g_object_set to pipeline thread**

**Problem**: HTTP thread calls `g_object_set` → acquires global GLib type lock → crashes → deadlock

**Fix**: Post bus message from HTTP thread, handle in pipeline thread
```cpp
// HTTP thread - just posts message (thread-safe)
gst_element_post_message(pipeline, message);

// Pipeline thread - handles message
g_signal_connect(bus, "message::application", callback);
// NOW in pipeline thread - safer for g_object_set
```

**Confidence: 40%** - Type lock might still be process-global

**B. Pipeline creation health check**

**Problem**: 50% stuck thread threshold missed production deadlock (5/14 = 35%)

**Fix**: Test if `gst_element_factory_make("fakesrc")` works
- Fork child process (inherits parent's locked mutexes via copy-on-write)
- Child tries to create element (requires global type lock + registry lock)
- If locked → child hangs → parent kills after 5s → test fails
- **Detects the ACTUAL failure**: "Would a new user session work?"

**Confidence: 90%** - Directly tests the failure condition

**C. Hourly core dumps**

**Problem**: If deadlock doesn't hit threshold or is restarted before watchdog triggers, no debugging data captured

**Fix**: Dump core every hour using gcore
- First dump at 5 minutes (verify it works)
- Then hourly
- Keeps last 48 hours (~360GB)
- gcore pauses process ~3s for atomic snapshot

**Why**: Production deadlocked at 42h with only 35% stuck (below threshold). Hourly dumps ensure we always have recent state.

**D. Complete debug symbols**

**Installed from ddebs.ubuntu.com plucky-updates:**
- ✅ libc6-dbg (pthread_mutex_lock, futex, epoll_wait)
- ✅ libglib2.0-0t64-dbgsym=2.84.1-1ubuntu0.1 (g_object_set, type system)
- ✅ libgstreamer1.0-0-dbgsym=1.26.0-3 (element factory, registry)
- ✅ gstreamer1.0-plugins-base-dbgsym=1.26.0-1ubuntu0.1
- ✅ gstreamer1.0-plugins-good-dbgsym=1.26.0-1ubuntu2.1

**All with security patches intact** (no CVE downgrades):
- libglib CVE-2025-4373 (Integer Overflow)
- gstreamer CVE-2025-47806/7/8 (DoS fixes)

Core dumps now show full backtraces through system libraries.

### 4. Dashboard Updates (Helix)

**Added to Wolf Health Panel:**
- 🔴 CRITICAL alert if `can_create_new_pipelines=false` (type lock held)
- 🟠 Warning if threads stuck but pipeline creation works (degraded but functional)
- 🟢 Success if healthy

**API updates:**
- Added `can_create_new_pipelines` field to `SystemHealthResponse`
- Watchdog uses this for CRITICAL detection (not 50% threshold)

### 5. Production Configuration

**Changes to production docker-compose.yaml:**
- ✅ Created `/opt/HelixML/wolf-debug-dumps/` directory
- ✅ Added volume mount: `./wolf-debug-dumps:/var/wolf-debug-dumps:rw`
- ⏳ Need to deploy new Wolf image with all fixes

### 6. Development Verification

**Wolf rebuilt and running with:**
- ✅ g_object_set fix (bus messaging)
- ✅ Pipeline creation health check (working, returns `true`)
- ✅ Hourly core dumps (first dump completed: 7.5GB at 16:03:44)
- ✅ Watchdog monitoring
- ✅ Full debug symbols (libc + GLib + GStreamer)

**Moonlight-web:**
- ✅ Re-paired with Wolf (deleted stale data.json)
- ✅ Streaming connections working

## GStreamer Global Locks

**Multiple process-global locks can cause similar deadlocks:**

1. **GLib Type System Lock** - `g_object_set()`, `g_object_new()`, element factory lookups
2. **GStreamer Registry Lock** - `gst_registry_find_feature()`, plugin lookups
3. **Plugin Loading Lock** - `.so` loading (rare after startup)

**Our health check tests BOTH type lock and registry lock:**
- `gst_element_factory_make("fakesrc")` needs registry lock (find factory) + type lock (instantiate)
- If either is held → test fails → new sessions won't work

## Watchdog Behavior

**Monitoring cycle** (every 30s):
1. Test pipeline creation (`gst_element_factory_make` with 5s timeout)
2. Count stuck threads (for context)
3. **CRITICAL if**: pipeline creation fails (not 50% threshold)
4. If CRITICAL for **>60s**:
   - Fork child, gather dumps (thread dump + gcore + logs)
   - **Parent waits for child**
   - Parent calls `exit(1)` → Docker restarts

**Dumps captured BEFORE restart** - verified in code.

## Honest Assessment

**Will the fix prevent production deadlocks?**
- **Maybe** - Thread confinement is good practice
- But type lock might still be process-global
- Need production testing for 42+ hours to verify
- **If it deadlocks again: GDB FIRST, restart SECOND**

**What would definitively prove/disprove the theory?**
- GDB attached to deadlocked process
- `thread apply all bt full` → see which thread holds type lock
- `info threads` → see mutex ownership
- **I destroyed this data by restarting** - critical mistake

## Production Deployment Checklist

When ready to deploy Wolf with fixes:

1. ✅ Verify debug dumps directory exists: `/opt/HelixML/wolf-debug-dumps/`
2. ✅ Verify volume mount in docker-compose.yaml
3. ⏳ Deploy new Wolf image (wolf-ui-working branch)
4. ⏳ Recreate Wolf container (picks up volume mount)
5. ⏳ Verify health API returns `can_create_new_pipelines: true`
6. ⏳ Verify first hourly dump created within 5 minutes
7. ⏳ Monitor for 48+ hours

**If deadlock occurs:**
1. **DO NOT RESTART** - follow CLAUDE.md GDB procedure
2. Collect full debugging data
3. Save to `/root/helix/design/2025-MM-DD-wolf-deadlock-gdb.txt`
4. Only then restart

## Files Changed

### Wolf Repository (wolf-ui-working branch)

- `src/moonlight-server/streaming/streaming.cpp`: Move g_object_set to pipeline thread
- `src/moonlight-server/wolf.cpp`: Watchdog + hourly dumps + pipeline health check
- `src/moonlight-server/monitoring/thread-monitor.hpp`: Pipeline creation test function
- `src/moonlight-server/api/api.hpp`: Add can_create_new_pipelines to response
- `src/moonlight-server/api/endpoints.cpp`: Call pipeline test, update status logic
- `docker/wolf.Dockerfile`: Debug symbols from ddebs.ubuntu.com (with security patches)

### Helix Repository (fix/add-wolf-healthcheck branch)

- `CLAUDE.md`: Critical debugging rules (NEVER restart without GDB)
- `design/2025-11-18-wolf-deadlock-root-cause.md`: Initial analysis
- `design/2025-11-18-wolf-deadlock-comprehensive-analysis.md`: Detailed investigation
- `design/2025-11-18-type-lock-theory-evidence.md`: Honest evidence assessment
- `design/2025-11-18-wolf-deadlock-final-summary.md`: This document
- `api/pkg/wolf/client.go`: Add can_create_new_pipelines field
- `frontend/src/components/wolf/WolfHealthPanel.tsx`: Display pipeline status
- `docker-compose.yaml` (production): Add debug dumps volume mount

## Key Lessons

1. **GDB before restart** - Hung processes contain irreplaceable debugging data
2. **Test actual failure conditions** - Not arbitrary percentages
3. **Fork inherits mutex state** - Brilliant way to test if lock is held
4. **Security patches matter** - Can't downgrade for debug symbols
5. **Global locks are catastrophic** - One crashed thread kills entire process
6. **Partial fixes are dangerous** - g_main_loop_quit fixed one deadlock, missed another

## Next Steps

**Immediate:**
- Deploy to production when ready
- Monitor pipeline creation test in dashboard
- Wait for first hourly dump (verify gcore works)

**If deadlock recurs:**
- GDB analysis per CLAUDE.md procedure
- Determine if fix worked (is it pipeline thread or HTTP thread?)
- Examine actual mutex state
- Prove or disprove type lock theory

**Long-term:**
- Consider removing interpipe (known buggy)
- Consider separate processes per pipeline (true fault isolation)
- Add mutex timeout detection (warn if acquiring >5s)
