# Wolf NVIDIA Driver and GStreamer Mutex Deadlock Analysis

**Date**: 2025-11-14
**System**: code.helix.ml production (Wolf container)
**Uptime**: 18+ days
**Symptom**: HTTPS endpoint (port 47984) completely hung, HTTP (port 47989) still working

## Executive Summary for Wolf Maintainers

Wolf process is deadlocked in NVIDIA proprietary driver code and GStreamer internal mutexes. This is **NOT** a simple blocking syscall issue - it's a complex multi-threaded deadlock involving GPU driver interaction with multimedia pipelines.

## Core Dump Evidence

### Deadlock #1: NVIDIA EGL Driver Mutex (0x705580003b80)

**Threads Affected:**
- **Main thread** (LWP 1342726) - Stuck in NVIDIA cleanup during exit
- **Thread 38** (LWP 1502538) - GStreamer audio producer thread

**Main Thread Backtrace:**
```
#0  __futex_lock_pi64 (futex_word=0x705580003b80) at futex-internal.c:180
#1  __pthread_mutex_lock_full (mutex=0x705580003b80)
#2-17 NVIDIA EGL library cleanup (libEGL_nvidia.so.0)
      ↑ Proprietary NVIDIA code - no symbols available
#18 __run_exit_handlers at exit.c:118
#19 __GI_exit (status=6)
#20 <signal handler> - SIGABRT (signal 6)
#21 shutdown_handler at exceptions.h:71
#22-28 pthread_join waiting for HTTP thread
#29 run() at wolf.cpp:231 - http_thread.join()
```

**Thread 38 Backtrace:**
```
#0  __futex_lock_pi64 (futex_word=0x705580003b80) at futex-internal.c:180
#1  __pthread_mutex_lock_full (mutex=0x705580003b80)
#2-? NVIDIA library code (symbols not available)
...
#3  __GI_ppoll
#4  libglib-2.0.so.0 (GLib main loop)
#5  g_main_loop_run
#6  streaming::run_pipeline (GStreamer pipeline execution)
#7  streaming::start_audio_producer (session_id="11136356655948363257")
```

**Analysis:**
- Two threads waiting on **same NVIDIA mutex**
- One thread is holding mutex 0x705580003b80 and never releasing it (identity unknown - core dump corrupted)
- This mutex is INSIDE NVIDIA's proprietary EGL library
- Deadlock likely caused by concurrent GPU operations from multiple GStreamer pipelines

### Deadlock #2: GStreamer Internal Mutex (0x70537c0062b0)

**Thread 99** (LWP 1345727):
```
#0  futex_wait (futex_word=0x70537c0062b0)
#1  __lll_lock_wait (mutex=0x70537c0062b0)
#3  ___pthread_mutex_lock (mutex=0x70537c0062b0)
#4  libgstbase-1.0.so.0 (GStreamer base library)
#5  gst_element_send_event (trying to send StopStreamEvent)
#6  g_main_loop_run
#7  streaming::run_pipeline
#8  streaming::start_audio_producer (session_id="9671ab72-d43e-4256-b48e-90ab13eb70d4")
```

**Analysis:**
- Separate GStreamer internal mutex deadlock
- Thread trying to send StopStreamEvent is blocked
- Indicates concurrent pipeline manipulation without proper synchronization

### Connection Leak

**Evidence:**
```bash
$ netstat -tnp | grep 47984 | grep CLOSE_WAIT | wc -l
17

$ lsof -p 1 | grep 47984
wolf  1 root 157u  IPv4  36010632  0t0  TCP *:47984->moonlight-web:47420 (CLOSE_WAIT)
... (16 more)
```

**Analysis:**
- 17 leaked connections over 18 days uptime
- CLOSE_WAIT = remote closed, but Wolf never called close()
- File descriptor leak (currently 555 open, limit 1048576)
- Accumulated resource pressure over time

### HTTPS Complete Hang vs HTTP Working

**HTTP (port 47989) - WORKING:**
```bash
$ curl -s http://localhost:47989/serverinfo
<?xml version="1.0" encoding="utf-8"?>
<root status_code="200"><hostname>Helix (code.helix.ml)</hostname>...
```

**HTTPS (port 47984) - HUNG:**
```bash
$ curl -v --max-time 15 -k https://localhost:47984/serverinfo
* TCP connection succeeds (SYN/ACK received)
* SSL "Client Hello" sent
* Wolf NEVER responds with "Server Hello"
* Timeout after 15 seconds
```

**Analysis:**
- NOT a global process hang
- HTTP event loop functional
- HTTPS SSL handshake stalled BEFORE completion callback
- SSL operations likely blocked on NVIDIA/GStreamer mutex contention

## Root Cause Hypothesis

### The Deadlock Chain

1. **Multiple GStreamer Pipelines** running concurrently (audio/video streaming)
2. **Each pipeline uses NVIDIA GPU** for hardware acceleration (CUDA, NVENC, etc.)
3. **NVIDIA driver has shared global mutex** for GPU resource management
4. **Thread A** (audio producer): Holds NVIDIA mutex, waiting on GStreamer mutex
5. **Thread B** (GStreamer event): Holds GStreamer mutex, waiting on NVIDIA mutex
6. **Classic circular deadlock** → both threads blocked forever

### Why HTTPS Hangs

- HTTPS SSL operations MAY use GPU acceleration (NVIDIA driver hooks into OpenSSL/BoringSSL)
- SSL handshake tries to acquire NVIDIA mutex
- Mutex already held by deadlocked GStreamer thread
- SSL handshake blocks forever waiting for mutex
- Client sees "handshake timeout"

## Why This Happened After 18 Days

1. **Connection leaks** accumulated resource pressure
2. **GPU memory fragmentation** from leaked sessions
3. **NVIDIA driver state corruption** from improper cleanup
4. **Increased lock contention** as resources became scarce
5. **Eventually reached critical mass** where mutex wait times exceeded timeouts

## Recommended Fixes

### IMMEDIATE (Deploy ASAP)

1. **Restart Wolf** - only way to clear deadlock
2. **Healthcheck** - auto-restart on hang (already implemented in Helix)
3. **Connection leak fix** - ensure proper socket close() in HTTPS server
4. **Increased timeouts** - moonlight-web 10s instead of 1s (already done)

### SHORT TERM (This Week)

1. **Session Cleanup Audit**:
   - Review streaming::start_audio_producer() cleanup path
   - Ensure GStreamer pipelines properly destroyed on StopStreamEvent
   - Add explicit NVIDIA resource cleanup (cuCtxSynchronize, cuCtxDestroy)

2. **Mutex Lock Ordering**:
   - Document lock acquisition order: GStreamer → NVIDIA
   - Add lock timeouts (pthread_mutex_timedlock) with 5s max wait
   - Log warnings on contention > 1s

3. **Connection Leak Fix**:
   - Find CLOSE_WAIT leak in SimpleWeb HTTPS server
   - Add connection lifecycle logging
   - Implement connection pool with max age

### MEDIUM TERM (2 Weeks)

1. **Separate GPU Context Per Pipeline**:
   - Each GStreamer pipeline gets isolated CUDA context
   - Prevents cross-pipeline mutex contention
   - Use CUDA MPS (Multi-Process Service) if available

2. **GStreamer Pipeline Synchronization**:
   - Single-threaded GStreamer bus/event handling
   - Queue StopStreamEvent instead of direct send
   - Drain queue before pipeline state changes

3. **NVIDIA Driver Workarounds**:
   - Disable GPU acceleration for SSL if possible
   - Use CPU-only crypto for HTTPS (acceptable performance hit)
   - Test with different NVIDIA driver versions

### LONG TERM (1 Month)

1. **Multi-Threaded HTTPS Server**:
   - io_service thread pool instead of single detached thread
   - Distribute SSL handshakes across worker threads
   - Reduce single-point-of-failure risk

2. **Resource Monitoring**:
   - Prometheus metrics for leaked connections
   - NVIDIA GPU memory/context tracking
   - Alert on mutex contention > threshold

3. **Graceful Degradation**:
   - Detect deadlock conditions early
   - Reject new connections instead of hanging
   - Circuit breaker pattern for HTTPS endpoint

## Debugging Artifacts

**Core Dump**: `/tmp/wolf-core.1342726` (17GB)
**Binary**: `/wolf/wolf`
**Analysis**: Most thread registers corrupted in core dump (gcore limitation)

**Successful Commands**:
```bash
# Generate core dump
gcore -o /tmp/wolf-core <PID>

# Analyze with symbols
docker exec helixml-wolf-1 gdb /wolf/wolf /tmp/wolf-core

# Thread backtraces
(gdb) thread apply all bt
(gdb) thread <N>
(gdb) bt full
```

## Questions for Wolf Maintainers

1. **NVIDIA GPU Usage**: Which Wolf components use NVIDIA GPU?
   - Video encoding (NVENC)?
   - CUDA for processing?
   - EGL for display capture?

2. **SSL GPU Acceleration**: Does Wolf/OpenSSL use NVIDIA for crypto?
   - Can this be disabled?
   - Is it necessary for performance?

3. **GStreamer Thread Safety**:
   - How many GStreamer pipelines run concurrently?
   - Are they isolated or share resources?
   - What's the locking strategy for state changes?

4. **Connection Lifecycle**:
   - Where should HTTPS connections be closed?
   - Is this in SimpleWeb library or Wolf code?
   - Any known issues with async_handshake cleanup?

## Related Issues

- Connection leak causing CLOSE_WAIT accumulation
- Possible NVIDIA driver bug in EGL mutex handling
- GStreamer concurrent pipeline manipulation
- SSL handshake blocking on external mutex

## Success Criteria After Fixes

- [ ] Zero NVIDIA mutex deadlocks under normal load
- [ ] Zero GStreamer mutex deadlocks
- [ ] Zero leaked connections (CLOSE_WAIT)
- [ ] SSL handshake completes in < 500ms (p95)
- [ ] System runs 30+ days without HTTPS hang
- [ ] Graceful degradation under resource pressure

---

**For Helix Team**: This analysis documents why Wolf hangs in production. The healthcheck fix provides temporary mitigation, but Wolf maintainers need to address the underlying NVIDIA/GStreamer deadlock issues.
