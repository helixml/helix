# Wolf Deadlock - What I Can PROVE vs What I'm Speculating

**Date**: 2025-11-14
**Uptime**: 16 hours 36 minutes (NOT 18 days - my error)
**Leak Rate**: 17 connections in 16.5 hours = ~1 per hour (RAPID failure, not gradual)

## PROVEN FACTS (From Core Dump Evidence)

### FACT #1: Thread 99 IS the HTTPS Server Thread

**Call Stack** (frames #28-32):
```
#32 clone()
#31 start_thread
#29 wolf.cpp:187 ← std::thread lambda for HTTPS server
#28 HTTPServers::startServer (port=47984) ← HTTPS PORT
#27 SimpleWeb::ServerBase::start
#26 io_context::run ← Boost ASIO event loop
```

**PROOF**: Thread 99 is the HTTPS server thread created at `wolf.cpp:185-188`.

### FACT #2: Thread 99 Was Processing /cancel Endpoint

**Call Stack** (frames #12-17):
```
#17 ServerBase::read ← Reading HTTPS request
#16 ServerBase::find_resource ← Matching URL to handler
#12 endpoints::https::cancel ← /cancel endpoint handler
```

**PROOF**: Thread was processing an HTTPS `/cancel` request when it got stuck.

### FACT #3: /cancel Handler Fired StopStreamEvent

**Call Stack** (frames #11-12):
```
#12 endpoints::https::cancel
#11 dp::event_bus::fire_event<StopStreamEvent> (event_bus.hpp:166)
#10 safe_shared_registrations_access (event_bus.hpp:232)
```

**PROOF**: The `/cancel` endpoint called `fire_event(StopStreamEvent)`.

### FACT #4: Event Bus Executed Handler Synchronously

**Event Bus Source Code** (`event_bus.hpp:165-178`):
```cpp
void fire_event(EventType&& evt) noexcept {
    safe_shared_registrations_access([this, local_event = std::forward<EventType>(evt)]() {
        for (auto [begin_evt_id, end_evt_id] = ...) {
            begin_evt_id->second(local_event);  // ← Direct function call
        }
    });
}
```

**Call Stack** (frame #9):
```
#9  fire_event lambda (event_bus.hpp:171) ← Executes handler
```

**PROOF**: Event bus executes handlers **synchronously** in caller's thread. Thread 99 (HTTPS) executed the StopStreamEvent handler.

### FACT #5: Handler Called gst_element_send_event

**Call Stack** (frames #5-8):
```
#8  std::__invoke_impl ← Invoking the lambda handler
#7  gst_element_send_event ← CALLED FROM HANDLER
#5  gst_element_send_event ← GStreamer library
#4  libgstbase-1.0.so.0 ← Trying to acquire mutex
```

**PROOF**: The StopStreamEvent handler (from `streaming.cpp:172-178`) called `gst_element_send_event()`.

### FACT #6: Thread 99 Blocked on GStreamer Mutex

**Call Stack** (frames #0-4):
```
#0  futex_wait (futex_word=0x70537c0062b0)
#1  __lll_lock_wait (mutex=0x70537c0062b0)
#2  lll_mutex_lock_optimized (mutex=0x70537c0062b0)
#3  ___pthread_mutex_lock (mutex=0x70537c0062b0)
#4  libgstbase-1.0.so.0 (internal GStreamer code)
```

**PROOF**: Thread 99 is permanently blocked waiting for mutex `0x70537c0062b0` inside GStreamer library code.

### FACT #7: No Connection Pool Exhaustion

**Evidence**:
```bash
$ lsof -p 1 | grep '47984.*LISTEN'
wolf  1 root  53u  IPv4  30397612  0t0  TCP *:47984 (LISTEN)

$ timeout 3 bash -c 'cat < /dev/null > /dev/tcp/localhost/47984'
SUCCESS  # ← TCP connection succeeds immediately
```

**PROOF**: HTTPS server listen socket is active and accepting new TCP connections. No connection pool limit.

### FACT #8: 17 Leaked Connections in CLOSE_WAIT

**Evidence**:
```bash
$ netstat -tnp | grep 47984 | grep CLOSE_WAIT
tcp  415   0  172.19.0.50:47984  162.142.125.39:31294  CLOSE_WAIT  -
tcp  518   0  127.0.0.1:47984    127.0.0.1:52258       CLOSE_WAIT  -
tcp  1547  0  172.19.0.50:47984  172.19.0.11:47794     CLOSE_WAIT  -
... (14 more)
```

**Sources**:
- 162.142.125.39: External browser clients
- 172.19.0.11: moonlight-web container
- 127.0.0.1: Localhost (debugging)

**PROOF**: Clients closed connections, Wolf received FIN, but never called `close()` on its side.

### FACT #9: HTTP Still Works, HTTPS Completely Hung

**Evidence**:
```bash
$ curl -s http://localhost:47989/serverinfo
<?xml version="1.0" encoding="utf-8"?>
<root status_code="200"><hostname>Helix (code.helix.ml)</hostname>...

$ curl -v --max-time 15 -k https://localhost:47984/serverinfo
* TCP connection: SUCCESS
* SSL Client Hello sent
* Wolf NEVER responds
* Timeout after 15s
```

**PROOF**: HTTP server functional, HTTPS server event loop deadlocked. NOT a global process hang.

### FACT #10: Main Thread Deadlocked on NVIDIA Mutex

**Call Stack**:
```
#0  __futex_lock_pi64 (futex_word=0x705580003b80)  ← NVIDIA mutex
#1  __pthread_mutex_lock_full (mutex=0x705580003b80)
#2-17 libEGL_nvidia.so.0 (proprietary NVIDIA code)
#18 __run_exit_handlers (exit cleanup)
#20 <signal handler> SIGABRT
```

**PROOF**: Main thread tried to exit (SIGABRT from gcore), got stuck in NVIDIA driver cleanup on mutex `0x705580003b80`.

### FACT #11: Thread 38 ALSO Waiting on NVIDIA Mutex

**Evidence**:
```
Thread 38 (LWP 1502538):
#0  __futex_lock_pi64 (futex_word=0x705580003b80)  ← SAME NVIDIA mutex
... (frames in NVIDIA library)
#6  streaming::run_pipeline (GStreamer audio producer)
```

**PROOF**: A GStreamer audio producer thread is ALSO waiting on the same NVIDIA mutex as main thread.

---

## WHAT I CANNOT PROVE

### SPECULATION #1: GStreamer Requires Main Loop Thread

**Claim**: "gst_element_send_event must be called from pipeline's g_main_loop_run thread"

**Evidence FOR**:
- Thread 99 deadlocked calling gst_element_send_event from HTTPS thread
- GStreamer mutex deadlock

**Evidence AGAINST**:
- Web search shows "event can be sent from application thread"
- GStreamer docs say it's "fully thread-safe"
- No explicit documentation saying "must call from main loop thread"

**VERDICT**: **UNPROVEN**. I have deadlock evidence but not proof this is inherently wrong.

### SPECULATION #2: Circular Deadlock Pattern

**Claim**: "Thread A holds X, waits for Y; Thread B holds Y, waits for X"

**Evidence FOR**:
- Thread 99 stuck on GStreamer mutex
- Only one thread waiting on that specific mutex
- Other thread must be holding it

**Evidence AGAINST**:
- Cannot identify which thread holds mutex 0x70537c0062b0
- Core dump too corrupted to see all thread states
- No proof of circular dependency

**VERDICT**: **UNPROVEN**. Plausible but no direct evidence.

### SPECULATION #3: NVIDIA SSL Crypto Acceleration

**Claim**: "SSL handshake uses NVIDIA GPU for crypto, holds NVIDIA mutex"

**Evidence FOR**:
- HTTPS specifically hangs (not HTTP)
- NVIDIA mutexes involved in deadlock
- Two threads waiting on same NVIDIA mutex

**Evidence AGAINST**:
- No proof OpenSSL compiled with NVIDIA support
- SSL handshake completes (TCP connect works, Client Hello sent)
- No evidence SSL operations in any backtrace

**VERDICT**: **UNPROVEN**. Speculative connection.

---

## ALTERNATIVE HYPOTHESES (Need Investigation)

### Hypothesis A: Recursive Locking

**Theory**: Thread 99 is trying to acquire a lock it ALREADY holds (recursive lock on non-recursive mutex).

**How to Test**:
- Check if event_bus `registration_mutex_` is held by Thread 99 while it's calling gst_element_send_event
- Check if GStreamer mutex 0x70537c0062b0 is recursive or non-recursive
- Examine stack for multiple lock acquisitions

**Evidence Needed**:
```bash
# In GDB, check mutex owner
(gdb) thread 99
(gdb) p *(pthread_mutex_t*)0x70537c0062b0
```

### Hypothesis B: Pipeline Already Destroyed

**Theory**: Pipeline `session_id="9671ab72..."` was already destroyed by its main loop thread. Thread 99 is trying to send event to invalid/destroyed pipeline.

**How to Test**:
- Find thread that ran `g_main_loop_run` for session 9671ab72
- Check if that thread exited
- Check pipeline object validity

**Evidence Needed**:
- Search core dump for session ID in other thread stacks
- Check if pipeline destructor was called

### Hypothesis C: GStreamer Bug (Mutex Not Released)

**Theory**: GStreamer internal bug - mutex was acquired but never released due to exception or early return.

**How to Test**:
- Check GStreamer version in Wolf container
- Search GStreamer bug tracker for similar deadlocks
- Check if upgrading GStreamer fixes it

**Evidence Needed**:
```bash
docker exec helixml-wolf-1 gst-inspect-1.0 --version
```

---

## WHAT I CAN ACTUALLY FIX (Without Full Proof)

### Fix #1: HTTPS Connection Leak (100% PROVEN)

**Evidence**: 17 connections in CLOSE_WAIT, error handler doesn't call close()
**Location**: `custom-https.cpp:18-26`
**Fix**: Add `socket->close()` in error handler
**Confidence**: **100% - This WILL eliminate CLOSE_WAIT leaks**

### Fix #2: Use g_main_loop_quit Instead of gst_element_send_event (80% Confidence)

**Evidence**: Thread 99 deadlocked calling gst_element_send_event
**Reasoning**: Even if GStreamer allows cross-thread calls, it's clearly causing deadlocks in practice
**Fix**: Replace with `g_main_loop_quit()` (documented as thread-safe)
**Confidence**: **80% - This SHOULD prevent deadlock, but I can't prove gst_element_send_event is the root cause**

**Why g_main_loop_quit is safer**:
- GLib documentation explicitly states it's thread-safe
- Doesn't acquire pipeline mutexes
- Lets pipeline clean up in its own thread context
- Eliminates cross-thread interaction entirely

---

## RECOMMENDED NEXT STEPS

### Option A: Deploy Fixes Without Full Proof

**Rationale**: Even without proving the exact mechanism, the fixes are sound:
1. Connection leak fix is objectively correct
2. Using thread-safe g_main_loop_quit is best practice regardless
3. Production is down - pragmatic fix needed

**Risk**: If my theory is wrong, deadlock might recur.

### Option B: Gather More Evidence First

**Methods**:
1. Install GStreamer debug symbols in container
2. Reanalyze core dump with symbols
3. Identify exact mutex owner
4. Prove circular dependency

**Risk**: Production stays down longer.

### Option C: Test Hypothesis Experimentally

**Method**:
1. Apply Fix #1 and #2 locally
2. Reproduce the deadlock with sustained load testing
3. If it doesn't recur, hypothesis validated
4. If it does recur, gather new core dump with more info

---

## HONEST ASSESSMENT

**What I've PROVEN**:
- ✅ HTTPS thread (Thread 99) called `gst_element_send_event` from `/cancel` handler
- ✅ Thread 99 is now permanently blocked on GStreamer mutex
- ✅ No connection pool exhaustion
- ✅ 17 leaked CLOSE_WAIT connections

**What I CANNOT prove**:
- ❌ Whether `gst_element_send_event` is inherently unsafe from other threads
- ❌ What thread holds the GStreamer mutex Thread 99 is waiting for
- ❌ Whether this is circular deadlock vs just long contention
- ❌ Whether NVIDIA is involved in the HTTPS hang (vs just shutdown artifact)

**My Theory** (unproven but plausible):
Calling `gst_element_send_event` from arbitrary threads causes GStreamer internal mutex contention that leads to deadlocks. Replacing with thread-safe `g_main_loop_quit()` eliminates the cross-thread interaction.

**Confidence Level**: 70% that my fixes will solve it, but I cannot prove the exact root cause mechanism.
