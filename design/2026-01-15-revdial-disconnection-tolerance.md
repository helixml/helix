# RevDial Disconnection Tolerance for TCP Stream Continuity

**Date:** 2026-01-15
**Status:** Phase 1 Implemented, Phase 2 Implemented
**Author:** Claude Code

## Problem Statement

The RevDial system currently provides reverse dial capability for NAT traversal, allowing the API server to initiate connections to clients behind NAT. However, when the RevDial control connection drops (network blip, load balancer timeout, client restart), all active TCP streams being forwarded over the connection are immediately terminated.

**Current Architecture:**

```
┌─────────────────┐                    ┌─────────────────┐
│   API Server    │                    │   RevDial       │
│                 │                    │   Client        │
│ ┌─────────────┐ │  Control (WS/HTTP) │ ┌─────────────┐ │
│ │   connman   │◄├────────────────────┼─┤  Listener   │ │
│ │             │ │                    │ │             │ │
│ │   Dialer    ├─┼──── Data (WS) ─────┼─►  ProxyConn  │ │
│ └─────────────┘ │                    │ └─────────────┘ │
└─────────────────┘                    └─────────────────┘
                                              │
                                              ▼
                                       ┌─────────────┐
                                       │ Local Svc   │
                                       │(desktop-    │
                                       │ bridge,     │
                                       │ hydra,etc)  │
                                       └─────────────┘
```

**Impact:**
- Screenshot/exec/clipboard requests fail if connection drops mid-request
- Video streaming (if routed via RevDial) gets interrupted
- Users experience service unavailability during brief network hiccups
- Load balancer idle timeouts (common at 60s) cause periodic disruptions

## RevDial Clients Inventory

There are three primary RevDial clients that need to be considered:

### 1. Hydra (`api/cmd/hydra/main.go`)
```go
revDialClient = revdial.NewClient(&revdial.ClientConfig{
    ServerURL:   revDialAPIURL,
    RunnerID:    "hydra-" + revDialSandboxID,
    LocalAddr:   "unix://" + socketPath,
})
```
**Traffic Pattern:** HTTP API calls to Hydra's Unix socket
- CreateDockerInstance, DeleteDockerInstance, ListDockerInstances
- Exec commands, file operations
- Short-lived request/response cycles

### 2. Desktop Bridge (`api/cmd/desktop-bridge/main.go`)
```go
revdialClient := revdial.NewClient(&revdial.ClientConfig{
    ServerURL:   apiURL,
    RunnerID:    runnerID,  // "desktop-{session_id}"
    LocalAddr:   fmt.Sprintf("localhost:%s", cfg.HTTPPort),
})
```
**Traffic Pattern:** HTTP API calls to desktop-bridge HTTP server
- Screenshots, clipboard, input events, file upload
- Mostly short-lived, but screenshots can be several MB

### 3. Standalone revdial-client (`api/cmd/revdial-client/main.go`)
```go
client := revdial.NewClient(&revdial.ClientConfig{
    ServerURL:   baseURL,
    RunnerID:    *runnerID,
    LocalAddr:   *localAddr,
})
```
**Traffic Pattern:** General TCP forwarding to arbitrary local address
- Could be used for any protocol
- Potentially long-lived streams

## Requirements

### Must Have
1. **Transparent reconnection** - Control connection can reconnect without application awareness
2. **In-flight request completion** - HTTP requests in progress should complete if connection restored quickly
3. **Connection state preservation** - API server should maintain `connman` entries across brief disconnections
4. **Backward compatible** - Existing clients should continue to work without modification

### Should Have
1. **Stream resumption** - Long-running TCP streams can resume after reconnect
2. **Buffered retransmission** - Small amount of data can be replayed after reconnect
3. **Graceful degradation** - If buffer exhausted, signal restart needed rather than corruption

### Nice to Have
1. **Zero data loss** - Complete stream continuity for arbitrarily long disconnections
2. **Connection migration** - Handle client IP change (mobile, VPN)

## Design Options

### Option A: Application-Level Retry (Minimal Change)

**Approach:** Don't maintain TCP stream continuity. Instead, ensure quick reconnection and rely on application-level retry.

**Server Changes:**
1. Add grace period to `connman` - don't remove dialer immediately on disconnect
2. Queue incoming Dial() requests during disconnection
3. When client reconnects, resume queued requests

**Client Changes:**
- None required (already has reconnect loop)

**Pros:**
- Simple implementation
- Works for current HTTP-based traffic patterns
- No protocol changes needed

**Cons:**
- In-flight requests fail and must be retried
- Long-running streams must restart from beginning
- Application must handle retry logic

**Effort:** Low (1-2 days)

### Option B: Session-Based Stream Multiplexing

**Approach:** Wrap each data connection in a session layer that can be resumed.

```
┌─────────────────────────────────────────────────────────────┐
│                    Session Layer                            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐    │
│  │Stream #1 │  │Stream #2 │  │Stream #3 │  │Stream #N │    │
│  │(pending) │  │(active)  │  │(active)  │  │(closed)  │    │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘    │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              Transport (WebSocket/TCP)                       │
│         (may disconnect and reconnect)                       │
└─────────────────────────────────────────────────────────────┘
```

**Protocol:**
```go
type SessionFrame struct {
    SessionID   uint32  // Identifies the logical stream
    SeqNum      uint64  // Sequence number for ordering/ack
    AckNum      uint64  // Last received sequence number
    Flags       uint8   // SYN, FIN, RST, ACK
    Payload     []byte  // Application data
}
```

**Server Changes:**
1. New `SessionManager` tracks logical sessions independent of transport
2. Buffer unacknowledged data (configurable, default 1MB per session)
3. On reconnect, replay unacknowledged data
4. Session timeout (e.g., 60s) for cleanup if no reconnect

**Client Changes:**
1. Mirror session tracking on client side
2. Send session ID with each data connection
3. Request retransmission on reconnect

**Pros:**
- True stream continuity for brief disconnections
- Works for any traffic pattern (HTTP, streaming, etc.)
- Graceful degradation when buffer exhausted

**Cons:**
- Significant protocol complexity
- Buffer memory usage
- Both client and server must be updated
- Potential for out-of-order delivery edge cases

**Effort:** High (1-2 weeks)

### Option C: Yamux Multiplexing over Control Connection

**Approach:** Use [yamux](https://github.com/hashicorp/yamux) (or similar) to multiplex all streams over the single control connection.

**Current Flow:**
```
Control Conn ─────►  "conn-ready" message  ─────► Client dials back
                                                         │
                                                         ▼
Data Conn #1  ◄───────────────────────────────  WebSocket to server
```

**New Flow:**
```
Control Conn ──► yamux session ──► multiple virtual streams
                      │
                      ├── Stream #1 (HTTP request)
                      ├── Stream #2 (HTTP request)
                      └── Stream #N (HTTP request)
```

**Server Changes:**
1. Wrap control connection in yamux.Session
2. `connman.Dial()` creates new yamux stream instead of requesting callback
3. Remove data connection webhook mechanism

**Client Changes:**
1. Accept yamux streams from server
2. Forward each stream to local service
3. No more dial-back required

**Pros:**
- Simpler flow (no dial-back complexity)
- Built-in keep-alive and flow control
- Natural reconnection boundary (reconnect = new yamux session)
- Battle-tested library

**Cons:**
- Still doesn't maintain streams across reconnection
- Requires updating all clients
- Changes fundamental RevDial architecture

**Effort:** Medium (3-5 days)

### Option D: Hybrid - Yamux + Session Resumption

**Approach:** Combine Options B and C - use yamux for multiplexing with a lightweight session resumption layer.

**Key Insight:** Yamux already provides:
- Stream multiplexing
- Flow control
- Keep-alive
- Graceful stream close

We add a thin session layer on top:
- Session IDs that persist across yamux sessions
- Small per-stream buffer (16KB) for retransmission
- Resume protocol on reconnect

**Server-side Session State:**
```go
type Session struct {
    ID           string
    RunnerID     string
    Created      time.Time
    LastActive   time.Time

    // Per-stream buffering (only for active streams)
    streams      map[uint32]*StreamState
}

type StreamState struct {
    LocalBuffer  *ring.Buffer  // Last 16KB sent to client
    RemoteAck    uint64        // Acknowledged by client
    Closed       bool
}
```

**Reconnection Protocol:**
1. Client connects with `Session-ID` header (or generates new)
2. Server checks for existing session
3. If exists and not expired:
   - Send buffered data from last ack point
   - Resume streams
4. If expired or missing:
   - Create new session
   - All streams restart

**Pros:**
- Best of both worlds
- Handles most real-world disconnection scenarios
- Bounded memory usage
- Graceful degradation

**Cons:**
- Most complex implementation
- Need careful edge case handling

**Effort:** High (1-2 weeks)

## Recommendation

**Start with Option A (Application-Level Retry)** as it provides immediate value with minimal risk, then evolve to **Option C (Yamux)** for better architecture.

### Phase 1: Graceful Reconnection (Option A)
**Goal:** Handle brief disconnections without failing in-flight requests

**Changes:**

1. **connman: Add reconnection grace period**
```go
type ConnectionManager struct {
    deviceDialers     map[string]*revdial.Dialer
    deviceConnections map[string]net.Conn
    disconnectedAt    map[string]time.Time  // Track when disconnected
    pendingDials      map[string][]chan dialResult  // Queue requests
    gracePeriod       time.Duration  // e.g., 30 seconds
    lock              sync.RWMutex
}

func (m *ConnectionManager) Dial(ctx context.Context, key string) (net.Conn, error) {
    m.lock.RLock()
    dialer, ok := m.deviceDialers[key]
    if ok {
        m.lock.RUnlock()
        return dialer.Dial(ctx)
    }

    // Check if recently disconnected (within grace period)
    disconnectTime, wasConnected := m.disconnectedAt[key]
    if wasConnected && time.Since(disconnectTime) < m.gracePeriod {
        // Queue this request and wait for reconnection
        m.lock.RUnlock()
        return m.waitForReconnect(ctx, key)
    }

    m.lock.RUnlock()
    return nil, ErrNoConnection
}
```

2. **connman: Handle reconnection**
```go
func (m *ConnectionManager) Set(key string, conn net.Conn) {
    m.lock.Lock()
    defer m.lock.Unlock()

    // Clear disconnection state
    delete(m.disconnectedAt, key)

    // Create new dialer
    m.deviceConnections[key] = conn
    m.deviceDialers[key] = revdial.NewDialer(conn, "/revdial")

    // Wake up any waiting Dial() calls
    if pending, ok := m.pendingDials[key]; ok {
        for _, ch := range pending {
            select {
            case ch <- dialResult{ready: true}:
            default:
            }
        }
        delete(m.pendingDials, key)
    }
}

func (m *ConnectionManager) OnDisconnect(key string) {
    m.lock.Lock()
    defer m.lock.Unlock()

    // Don't remove immediately - start grace period
    m.disconnectedAt[key] = time.Now()

    // Close old dialer but keep the key reserved
    if dialer, ok := m.deviceDialers[key]; ok {
        dialer.Close()
        delete(m.deviceDialers, key)
    }
}
```

3. **revdial: Detect disconnection and notify connman**
   - Add callback/hook for connection close
   - Call `connman.OnDisconnect()` instead of `Remove()`

**Effort:** 1-2 days

### Phase 2: Video/Input Stream Resilience (Current Focus)
**Goal:** Keep long-lived WebSocket streams (video/input) alive across RevDial reconnections

**Current Video/Input Architecture:**
```
Browser ─WebSocket─► API Server ─RevDial─► desktop-bridge ─► desktop-server
          (HTTPS)     (proxy)     (WS)        (HTTP)         (GNOME D-Bus)
```

The API's `proxyInputWebSocket` and `proxyStreamWebSocket` handlers:
1. Hijack the browser connection to get raw `net.Conn`
2. Call `connman.Dial()` to get RevDial connection to desktop
3. Forward WebSocket upgrade through RevDial
4. Start bidirectional `io.Copy` between browser and desktop

**Problem:** When RevDial dies, the `io.Copy` fails and both connections close.

**Solution: Resilient WebSocket Proxy**

Replace the simple bidirectional copy with a session-aware proxy that:
1. Assigns a session ID to each stream
2. Buffers data during brief disconnections
3. Re-establishes desktop-side WebSocket on reconnect
4. Resumes data flow

**API-Side Implementation (`ResilientProxy`):**

```go
type ResilientProxy struct {
    sessionID    string
    dialFunc     DialFunc    // Function to dial server (via connman)
    upgradeFunc  UpgradeFunc // Function to upgrade connection to WebSocket
    bufferSize   int

    clientConn net.Conn // Browser connection (stable)
    serverConn net.Conn // Server connection (may reconnect)
    serverMu   sync.Mutex

    // Bidirectional buffering (512KB each direction)
    inputBuffer    []byte     // client → server
    inputBufferMu  sync.Mutex
    inputBufferPos int

    outputBuffer    []byte    // server → client
    outputBufferMu  sync.Mutex
    outputBufferPos int

    // State
    reconnecting   atomic.Bool
    closed         atomic.Bool
    done           chan struct{}
    reconnectDone  chan struct{} // Signals reconnection completed
    reconnectMu    sync.Mutex    // Protects reconnection initiation
    serverError    chan error    // Channel to signal server errors
}

func (p *ResilientProxy) Run(ctx context.Context) error {
    // Start both copy goroutines - run for proxy lifetime
    go func() { clientToServerErr <- p.copyClientToServer(ctx) }()
    go func() { serverToClientErr <- p.copyServerToClient(ctx) }()

    // Main loop handles reconnection
    for {
        select {
        case err := <-p.serverError:
            // Server error - attempt reconnection
            if err := p.reconnect(ctx); err != nil {
                return fmt.Errorf("reconnection failed: %w", err)
            }
            // Copy goroutines resume automatically
        case err := <-clientToServerErr:
            return err // Fatal error
        case err := <-serverToClientErr:
            return err // Fatal error
        }
    }
}

func (p *ResilientProxy) copyClientToServer(ctx context.Context) error {
    for {
        n, err := p.clientConn.Read(buf)
        if p.reconnecting.Load() {
            p.bufferInput(buf[:n])  // Buffer during reconnection
            p.waitForReconnect(ctx) // Wait for reconnection to complete
            continue
        }
        p.serverConn.Write(buf[:n])
    }
}

func (p *ResilientProxy) copyServerToClient(ctx context.Context) error {
    for {
        if p.reconnecting.Load() {
            p.waitForReconnect(ctx) // Wait during reconnection
            continue
        }
        n, err := p.serverConn.Read(buf)
        if err != nil {
            p.signalServerError(err) // Trigger reconnection
            p.waitForReconnect(ctx)
            continue
        }
        p.clientConn.Write(buf[:n])
    }
}
```

**Desktop-Bridge Side Implementation:**

The desktop-bridge's `ProxyConn` also needs to be resilient:
1. When RevDial connection dies, keep local connection open (if possible)
2. Buffer data from local service
3. When RevDial reconnects (new data connection), resume

```go
type ResilientLocalProxy struct {
    localConn     net.Conn  // Connection to local service
    remoteConn    net.Conn  // RevDial connection (may reconnect)

    // Buffer for local→remote direction (video frames)
    outputBuffer  *ring.Buffer  // But for video, we want latest frame only

    // Session ID for correlation
    sessionID     string
}
```

**Bidirectional Buffering Rationale:**

The implementation uses symmetric 512KB buffers for both directions:

**Input (client → server):**
- Buffer all input events during disconnection
- Flush buffered events after reconnection
- 512KB is ample for input (~5000+ events)

**Output (server → client):**
- Output buffer exists for symmetry but rarely used
- When server disconnects, there's no data to buffer (source is down)
- After reconnection, video resumes from new data
- Clean termination on buffer overflow prevents data corruption

**Key Design Decision:** Instead of handling keyframes/IDR for video resynchronization, we terminate cleanly when buffer overflows. This means:
- No data gaps or corruption in the stream
- Browser sees clean connection close, can reconnect
- Simpler implementation, no protocol changes needed

**Session Correlation:**

When desktop-bridge reconnects via RevDial, we need to resume the correct session:
1. Add session ID to the WebSocket upgrade request
2. API tracks active sessions by ID
3. On reconnect, match session ID to resume correct stream

### Phase 3: Yamux Migration (Future)
**Goal:** Simplify architecture by multiplexing all streams over control connection

Benefits:
- Single connection to monitor
- Built-in flow control and keepalive
- Simpler reconnection boundary

This can be done after Phase 2 proves the concept works.

## Implementation Plan

### Phase 1 Tasks ✅ COMPLETED

1. **Add grace period to connman** ✅
   - Added `disconnectedAt` map
   - Added `gracePeriod` config (default 30s)
   - Added `OnDisconnect()` method

2. **Add dial queuing** ✅
   - Added `pendingDials` map
   - Implemented `waitForReconnect()` with context timeout
   - Wake pending callers on `Set()`

3. **Detect and handle disconnection** ✅
   - Added `watchDialer()` to monitor `Dialer.Done()` channel
   - Automatic callback to `OnDisconnect()` when dialer closes
   - No revdial package changes needed

4. **Testing** ✅
   - Unit tests for all reconnection scenarios in `connman_test.go`
   - Tests for grace period, context cancellation, max pending limits

**Files changed:**
- `api/pkg/connman/connman.go` - Major enhancement with grace period support
- `api/pkg/connman/connman_test.go` - New test file

### Phase 2 Tasks ✅ COMPLETED (Video/Input Stream Resilience)

1. **Create ResilientProxy for API-side** ✅
   - Created `api/pkg/proxy/resilient.go`
   - Implemented `ResilientProxy` struct with:
     - **Bidirectional buffering** - 512KB buffer for each direction (input and output)
     - Reconnection logic using connman.Dial()
     - WebSocket re-upgrade on reconnect via `UpgradeFunc`
     - Clean termination on buffer overflow (no data corruption)
     - Buffer clearing on Close() to prevent data leakage
   - Replaced bidirectional copy in `proxyInputWebSocket` and `proxyStreamWebSocket`

2. **Add session tracking** ✅
   - Generate unique proxy session ID for each connection (`generateProxySessionID()`)
   - Logging includes session ID for debugging

3. **Bidirectional buffering implementation** ✅
   - `copyClientToServer()` - Buffers input data during reconnection
   - `copyServerToClient()` - Waits during reconnection, resumes after
   - Both goroutines run for proxy lifetime, handling reconnection internally
   - `signalServerError()` coordinates reconnection between both directions
   - `waitForReconnect()` blocks until reconnection completes

4. **Graceful timeout behavior** ✅
   - 30 second reconnection timeout
   - 3 max reconnection attempts with backoff
   - Buffer overflow terminates cleanly (no gap/corruption in stream)
   - No keyframe handling needed since stream terminates cleanly on overflow

5. **Testing** ✅
   - Unit tests pass for connman reconnection scenarios
   - Build succeeds with all changes

**Files changed:**
- `api/pkg/proxy/resilient.go` - New package with ResilientProxy
- `api/pkg/server/external_agent_handlers.go` - Updated to use ResilientProxy

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Stale connections in grace period waste resources | Low | Limit grace period (30s) |
| Pending dials accumulate during long outage | Medium | Max pending limit + timeout |
| Race conditions in reconnection | High | Careful locking, thorough testing |
| Backward compatibility | Medium | Version handshake, fallback to immediate remove |

## Open Questions

1. ~~**Grace period duration?**~~ ✅ Resolved: 30 seconds default, configurable via `NewWithGracePeriod()`

2. ~~**Max pending dials?**~~ ✅ Resolved: 100 pending per runner, configurable constant

3. **Should we expose reconnection status?** Could add `/health` endpoint or metric showing runners in grace period. (Low priority)

4. ~~**Video streaming path?**~~ ✅ Clarified: Video/input goes through RevDial, Phase 2 addresses this.

5. ~~**Input buffer size for Phase 2?**~~ ✅ Resolved: 512KB per direction (configurable via `BufferSize` in `ResilientProxyConfig`). This provides ample buffering for brief disconnections while terminating cleanly on overflow.

6. ~~**How to signal keyframe request?**~~ ✅ Not needed: The design terminates cleanly on buffer overflow rather than allowing data gaps, so no keyframe resynchronization is required.

7. **Desktop-side connection lifetime?** When RevDial dies, should desktop-bridge:
   - Keep local connection open indefinitely waiting for reconnect?
   - Timeout after grace period (30s)?
   - Close immediately and re-establish on reconnect?

   **Current implementation:** API-side uses connman.Dial() which queues during grace period. Desktop-bridge side may need updates for full session resumption (Phase 3 consideration).

## Metrics to Add

```go
// Prometheus metrics
revdial_reconnection_grace_period_entries  // Number of runners in grace period
revdial_reconnection_success_total         // Successful reconnections within grace
revdial_reconnection_timeout_total         // Reconnections that timed out
revdial_pending_dials_current              // Current pending Dial() calls
revdial_pending_dials_total                // Total Dial() calls that had to wait
```

## References

- [hashicorp/yamux](https://github.com/hashicorp/yamux) - Stream multiplexing library
- [Mosh](https://mosh.org/) - Inspiration for session continuity over unreliable networks
- [QUIC](https://www.chromium.org/quic/) - Connection migration and 0-RTT resumption
- Existing design: `design/2025-11-24-revdial-implementation-complete.md`
