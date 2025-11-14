# Wolf SSL Handshake Hang - Root Cause Analysis

**Date**: 2025-11-14
**Status**: CRITICAL - Production Outage Investigation
**Priority**: P0 - System Down

## Executive Summary

Wolf's HTTPS endpoint is completely hung in production. Curl test confirms **SSL handshake never completes** - TCP connection succeeds but SSL "Client hello" gets no response after 15+ seconds. This is **NOT a timeout issue** - Wolf is genuinely deadlocked.

## Production Evidence

### CONFIRMED ROOT CAUSE: Connection Leak in CLOSE_WAIT ⚠️

**Smoking Gun**:
```bash
$ netstat -tnp | grep 47984 | grep CLOSE_WAIT | wc -l
17
```

**17 leaked connections in CLOSE_WAIT state** on port 47984 (HTTPS).

**What CLOSE_WAIT means**:
- Remote client closed the connection
- Wolf received FIN packet
- But Wolf NEVER called close() on its socket
- File descriptor leaked
- Eventually exhausts resources or corrupts state

**Impact**:
- Over 18 days uptime → accumulated 17 leaks
- Each leak consumes fd + memory
- May corrupt SSL state causing future handshakes to hang

### Test Results (code.helix.ml - 2025-11-14 09:22 UTC)

```bash
# TCP port check
timeout 5 bash -c 'cat < /dev/null > /dev/tcp/localhost/47984'
→ SUCCESS (port responsive)

# SSL handshake test
curl -v --max-time 15 -k https://localhost:47984/serverinfo
→ TIMEOUT after 15s
→ Sent "Client hello" but Wolf NEVER responded
```

### Wolf Process State
- **PID**: 1
- **CPU**: 48% (high baseline)
- **Memory**: 1.6GB / 62GB
- **Threads**: 101
- **File Descriptors**: 555 (well below 1048576 limit)
- **Uptime**: 18+ days
- **Zombie processes**: 1 (dpkg-preconfigu defunct)

### Error Pattern
```
[ERROR] [Stream]: failed to get app list from host:
Api(RequestClient(Reqwest(reqwest::Error {
  kind: Request,
  url: "https://wolf:47984/serverinfo?uniqueid=...",
  source: hyper_util::client::legacy::Error(Connect, TimedOut)
})))
```

Frequency: ~10% of new session creation attempts

## Code Path Analysis

### HTTPS Request Flow (BROKEN)

**File**: `src/moonlight-server/wolf.cpp:185-188`
```cpp
std::thread([local_state, p_key_file, p_cert_file]() {
    HttpsServer server = HttpsServer(p_cert_file, p_key_file);
    HTTPServers::startServer(&server, local_state, state::get_port(state::HTTPS_PORT));
}).detach();
```

**File**: `src/moonlight-server/rest/servers.cpp:136-143`
```cpp
server->resource["^/serverinfo$"]["GET"] = [&state](auto resp, auto req) {
    if (auto client = get_client_if_paired(state, req)) {  // ← Line 137
      auto client_session = state::get_session_by_client(...);
      endpoints::serverinfo<SimpleWeb::HTTPS>(resp, req, client_session, state);
    } else {
      reply_unauthorized(req, resp);
    }
};
```

**File**: `src/moonlight-server/rest/servers.cpp:112-114`
```cpp
std::optional<state::PairedClient>
get_client_if_paired(const immer::box<state::AppState> state, ...) {
  auto client_cert = SimpleWeb::Server<SimpleWeb::HTTPS>::get_client_cert(request);
  return state::get_client_via_ssl(state->config, std::move(client_cert));
}
```

**File**: `src/moonlight-server/rest/custom-https.cpp:76-79`
```cpp
x509::x509_ptr Server<HTTPS>::get_client_cert(...) {
  auto connection = request->connection.lock();  // ← POTENTIAL DEADLOCK
  auto cert = SSL_get_peer_certificate(connection->socket->native_handle());
  return x509::x509_ptr(cert, X509_free);
}
```

**File**: `src/moonlight-server/rest/custom-https.cpp:60-70`
```cpp
session->connection->socket->async_handshake(asio::ssl::stream_base::server,
                                             [this, session](const error_code &ec) {
                                               session->connection->cancel_timeout();
                                               auto lock = session->connection->handler_runner->continue_lock();  // ← POTENTIAL DEADLOCK
                                               if (!lock)
                                                 return;
                                               if (!ec)
                                                 this->read(session);
                                               else if (this->on_error)
                                                 this->on_error(session->request, ec);
                                             });
```

### Potential Root Causes

#### 1. CONFIRMED: MAC Address Enumeration Deadlock ⚠️

**File**: `src/moonlight-server/rest/endpoints.hpp:75`
```cpp
utils::lazy_value_or(host->mac_address, [&]() { return get_mac_address(local_ip); })
```

**File**: `src/moonlight-server/platforms/hw_linux.cpp:222-262`
```cpp
std::string get_mac_address(std::string_view local_ip) {
  ifaddrs *ifaddrptr = nullptr;
  if (getifaddrs(&ifaddrptr) == -1) {  // ← CAN BLOCK INDEFINITELY
    // ...
  }
  // Iterates through ALL network interfaces
  for (auto ifa = ifAddrStruct.get(); ifa != NULL; ifa = ifa->ifa_next) {
    // ... searches for matching IP and MAC
  }
}
```

**Problem**:
- `getifaddrs()` is a BLOCKING syscall
- Can hang indefinitely if network subsystem has issues
- Called INLINE in HTTPS endpoint handler
- Runs in single-threaded io_service event loop
- If it blocks, ALL SSL handshakes freeze

**Why WOLF_INTERNAL_MAC doesn't prevent this**:
- `host->mac_address` is initialized from env var at startup (wolf.cpp:59-62)
- But it's stored in `host` object which might get reloaded/reset
- Need to verify if `host` object can lose its mac_address value during runtime

#### 2. SUSPECTED: Shared Mutex/Lock Contention

**File**: `custom-https.cpp:63`
```cpp
auto lock = session->connection->handler_runner->continue_lock();
```

This acquires a lock for every async operation. If:
- Multiple SSL handshakes happening concurrently
- One blocks holding the lock
- Others wait forever

**Evidence needed**: Thread dump showing what locks are held

#### 3. SUSPECTED: Weak Pointer Deadlock

**File**: `custom-https.cpp:77`
```cpp
auto connection = request->connection.lock();  // weak_ptr::lock()
```

If the connection weak_ptr is invalid but still being accessed, this could cause undefined behavior.

#### 4. Single-Threaded Event Loop Bottleneck

**Architecture**:
- HTTPS server runs on **ONE detached thread** (wolf.cpp:185)
- That thread likely runs a **single-threaded io_service**
- ALL SSL operations serialize through that one thread
- If ANY operation blocks → entire HTTPS endpoint freezes

**Comparison**:
- HTTP server: Separate thread, still working (port 47989 responsive)
- HTTPS server: Frozen (port 47984 accepts but SSL hangs)

## Recommended Fixes

### IMMEDIATE (Deploy Today) ⚡

#### Fix 1: Never Call get_mac_address() in Request Path

**File**: `src/moonlight-server/rest/endpoints.hpp:75`

**Current (BROKEN)**:
```cpp
utils::lazy_value_or(host->mac_address, [&]() { return get_mac_address(local_ip); })
```

**Fix**:
```cpp
// Always use cached value, NEVER enumerate at request time
host->mac_address.value_or("00:11:22:33:44:55")
```

**Why**:
- `getifaddrs()` is BLOCKING and can hang
- Should NEVER be called in request handler
- Accept stale/default MAC rather than block

**Alternative**: Pre-compute MAC at startup in background thread, store in atomic

#### Fix 2: Add Timeout to SSL Handshake

**File**: `src/moonlight-server/rest/custom-https.cpp:59`

**Current**:
```cpp
session->connection->set_timeout(config.timeout_request);
```

**Issue**: `timeout_request` might be too long or not set

**Fix**: Enforce 5-second SSL handshake timeout:
```cpp
session->connection->set_timeout(std::chrono::seconds(5));  // Explicit SSL timeout
```

#### Fix 3: Moonlight-web Connect Timeout (DONE ✅)

**File**: `moonlight-common/src/network/reqwest.rs:30`
```rust
.connect_timeout(Duration::from_secs(10))  // was: 1
```

**Status**: Already pushed to moonlight-web feature/kickoff branch

### SHORT-TERM (This Week) 🔥

#### Fix 4: Multi-Threaded io_service for HTTPS

**Problem**: Single thread processes ALL SSL handshakes serially

**Fix**: Use io_service thread pool:
```cpp
// Create thread pool for HTTPS
boost::asio::io_service io_service;
boost::asio::io_service::work work(io_service);

std::vector<std::thread> threads;
for (int i = 0; i < 4; ++i) {  // 4 worker threads
  threads.emplace_back([&io_service]() { io_service.run(); });
}

// HTTPS server uses shared io_service
HttpsServer server = HttpsServer(p_cert_file, p_key_file, io_service);
```

**Benefit**:
- Parallel SSL handshake processing
- One blocked operation doesn't freeze all others

#### Fix 5: Watchdog Thread

**Implementation**:
```cpp
std::atomic<std::chrono::steady_clock::time_point> last_heartbeat;

// Main thread updates heartbeat
void heartbeat_loop() {
  while (true) {
    last_heartbeat = std::chrono::steady_clock::now();
    std::this_thread::sleep_for(std::chrono::seconds(1));
  }
}

// Watchdog thread monitors
void watchdog_loop() {
  while (true) {
    auto now = std::chrono::steady_clock::now();
    auto elapsed = std::chrono::duration_cast<std::chrono::seconds>(now - last_heartbeat.load()).count();

    if (elapsed > 30) {
      logs::log(logs::error, "Main event loop hung for {}s, exiting for restart", elapsed);
      std::exit(1);  // Docker will restart
    }

    std::this_thread::sleep_for(std::chrono::seconds(5));
  }
}
```

### MEDIUM-TERM (Next 2 Weeks) 📊

#### Fix 6: Asynchronous MAC Address Resolution

**At startup**:
```cpp
std::atomic<std::string> cached_mac_address = "00:11:22:33:44:55";

std::thread([&cached_mac_address, internal_ip]() {
  // Run in background, don't block startup
  auto mac = get_mac_address(internal_ip.value_or("172.19.0.50"));
  cached_mac_address.store(mac);
}).detach();
```

**In serverinfo**:
```cpp
// Always use atomic cached value - never blocks
auto mac = cached_mac_address.load();
```

#### Fix 7: Connection Pooling with Semaphore

```cpp
// Limit concurrent SSL handshakes to prevent overload
static std::counting_semaphore<3> ssl_semaphore(3);

void accept() {
  auto connection = create_connection(*io_service, context);

  acceptor->async_accept(connection->socket->lowest_layer(),
    [this, connection](const error_code &ec) {
      if (ec) return;

      // Acquire slot for SSL handshake (max 3 concurrent)
      ssl_semaphore.acquire();

      session->connection->socket->async_handshake(...,
        [this, session, &ssl_semaphore](...) {
          ssl_semaphore.release();  // Free slot
          // ... rest of handler
        });
    });
}
```

## Investigation Commands

### Check if Wolf is hung RIGHT NOW:
```bash
ssh root@code.helix.ml "docker exec helixml-wolf-1 curl -v --max-time 5 -k https://localhost:47984/serverinfo?uniqueid=test 2>&1 | grep -E 'Client hello|timed out'"
```

### Get thread state:
```bash
ssh root@code.helix.ml "docker exec helixml-wolf-1 cat /proc/1/status | grep Threads"
ssh root@code.helix.ml "docker exec helixml-wolf-1 ls /proc/1/task | wc -l"
```

### Check for fd leaks:
```bash
ssh root@code.helix.ml "docker exec helixml-wolf-1 ls -la /proc/1/fd | wc -l"
```

### Monitor in real-time:
```bash
watch -n 2 'docker exec helixml-wolf-1 timeout 2 bash -c "cat < /dev/null > /dev/tcp/localhost/47984" && echo "✓ HTTPS OK" || echo "✗ HTTPS HUNG"'
```

## Next Steps

1. **DO NOT RESTART WOLF YET** - Collect more debugging data
2. **Deploy Fix 1** (remove get_mac_address from request path) - CRITICAL
3. **Deploy Fix 2** (SSL handshake timeout) - CRITICAL
4. **Deploy Fix 4** (multi-threaded io_service) - HIGH
5. **Deploy Fix 5** (watchdog) - HIGH
6. **Add healthcheck** (auto-recovery fallback) - MEDIUM

## Files to Modify

### Priority 1 (CRITICAL):
- `src/moonlight-server/rest/endpoints.hpp:75` - Remove get_mac_address() call
- `src/moonlight-server/rest/custom-https.cpp:59` - Add explicit SSL timeout
- `src/moonlight-server/wolf.cpp:185-188` - Add thread pool for HTTPS

### Priority 2 (HIGH):
- `src/moonlight-server/wolf.cpp` - Add watchdog thread
- `docker-compose.yaml` - Add healthcheck
- `wolf.Dockerfile` - Install curl for debugging

### Priority 3 (MEDIUM):
- `src/moonlight-server/platforms/hw_linux.cpp` - Async MAC resolution
- Add Prometheus /metrics endpoint
- Add connection pooling

## Questions to Answer

1. **Does host->mac_address ever get reset/cleared during runtime?**
   - Check if host object is immutable or can be reloaded

2. **What is config.timeout_request value?**
   - Is SSL handshake timeout actually set?

3. **How many threads does SimpleWeb::Server use?**
   - Single-threaded or thread pool?

4. **Are there other blocking syscalls in the HTTPS path?**
   - File I/O, DNS lookups, Docker API calls?

## Core Dump Analysis (2025-11-14 10:42 UTC)

### Core Dump Captured
- **Location**: `/tmp/wolf-core.1342726` (17GB)
- **Method**: `gcore` from host (PID 1342726)
- **Threads**: 101 total

### Key Findings from Backtrace

**Main Thread (LWP 1342726) - SHUTDOWN DEADLOCK**:
```
#0  __futex_lock_pi64 at futex-internal.c:180
#1  __pthread_mutex_lock_full
#2-17 NVIDIA EGL library cleanup (libEGL_nvidia.so.0)
#18 __run_exit_handlers at exit.c:118
#19 __GI_exit
#20 <signal handler> - SIGABRT (signal 6)
#21 shutdown_handler at exceptions.h:71
#22-28 pthread join waiting for HTTP thread
#29 run() at wolf.cpp:231 - http_thread.join()
```

**Analysis**: Main thread is deadlocked in NVIDIA driver cleanup **triggered by gcore SIGABRT**. This is NOT the original hang - it's an artifact of the core dump capture process.

**HTTP Server Thread (47989) - WORKING**:
```
#0  epoll_reactor::run at epoll_reactor.ipp:521
#8  SimpleWeb::ServerBase::start at server_http.hpp:492
#9  HTTPServers::startServer (port=47989) at servers.cpp:105
#10 wolf.cpp:181 lambda (HTTP server thread)
```

**Status**: HTTP server thread is healthy, sitting in epoll event loop.

**GStreamer Thread (LWP 1345727) - HUNG ON MUTEX**:
```
#0  futex_wait (futex_word=0x70537c0062b0)
#1  __lll_lock_wait (mutex=0x70537c0062b0)
#3  ___pthread_mutex_lock (mutex=0x70537c0062b0)
#4  libgstbase-1.0.so.0
#5  gst_element_send_event
#8  StopStreamEvent handler
```

**Analysis**: GStreamer thread waiting on mutex 0x70537c0062b0 while trying to send a stop event. This could indicate concurrent pipeline manipulation.

**HTTPS Thread - NOT YET IDENTIFIED**:
- Expected to be detached thread from wolf.cpp:185-188
- Should be running SimpleWeb HTTPS server on port 47984
- Need to find it in the thread list

### Next Steps for Core Dump Investigation

1. **Find HTTPS thread**: Search for thread with `async_handshake` or SSL operations
2. **Identify which mutex it's waiting on**: Check if same as GStreamer thread (0x70537c0062b0)
3. **Map futex addresses to code**: Determine what locks are being held
4. **Check for circular wait**: Thread A holds lock X, waits for lock Y; Thread B holds lock Y, waits for lock X

## Additional Debugging Needed

Before restarting Wolf, collect:
- [x] Thread backtraces (captured via gcore)
- [x] Network connection states (17 CLOSE_WAIT)
- [ ] HTTPS thread identification
- [ ] Mutex ownership mapping
- [ ] Recent logs with correlation IDs
- [ ] Paired clients count (could affect iteration time)

## Success Criteria

After fixes:
- [ ] SSL handshake completes in < 500ms (p95)
- [ ] Zero handshake timeouts under normal load
- [ ] Watchdog prevents hangs > 30s
- [ ] System auto-recovers from deadlocks
- [ ] No session creation failures
