# Requirements: End-to-End Tests for RevDial Connectivity

## Background

RevDial is the reverse-dial tunnel used by sandboxes and external agents to connect back to the Helix API. The sandbox connects **out** to `/api/v1/revdial` via WebSocket; the API then dials **back** through that tunnel to reach Hydra, desktop-bridge, MCP, and other services running inside the container.

Current unit tests cover `Dialer` internals and `ConnectionManager` grace-period logic in isolation, using mock connections or `net.Pipe()`. There are no tests that exercise the full round-trip with real WebSocket traffic, nor tests for the `Client` reconnection logic.

---

## User Stories

### US-1 — Full round-trip smoke test
As a developer, I want a test that starts a real RevDial control server, connects the RevDial `Client` to it, and then sends an HTTP request through the tunnel to a local service — so I can verify the data path works end-to-end.

**Acceptance criteria:**
- A test server listens on a random port and handles WebSocket upgrades for both control and data connections.
- A `revdial.Client` connects to the test server (control WebSocket).
- The server registers the connection in a `connman.ConnectionManager`.
- A plain HTTP server listens on a local port as the "backend" inside the tunnel.
- `connman.Dial()` opens a connection through the tunnel; an HTTP request proxied over it reaches the backend and returns the expected response.
- The test completes in under 5 seconds.

### US-2 — Client utility unit tests
As a developer, I want tests for the pure helper functions in `client.go` so regressions are caught immediately without a running server.

**Acceptance criteria:**
- `ExtractHostAndTLS` is tested for `http://`, `https://`, `ws://`, `wss://` schemes, with and without explicit ports.
- `DialLocal` is tested for both TCP (`localhost:N`) and Unix socket (`unix:///tmp/sock`) targets using a local listener.

### US-3 — Reconnection test
As a developer, I want a test that drops the control WebSocket and verifies the client reconnects and the tunnel becomes usable again — so I trust the auto-reconnect loop works.

**Acceptance criteria:**
- The test server closes the control WebSocket after the first connection.
- The `Client`'s reconnect loop re-establishes a new control connection.
- After reconnection, a data request through the tunnel succeeds.
- The `reconnectDelay` used in the test is short (≤ 100 ms) so the test finishes in under 3 seconds.

### US-4 — `ProxyConn` data-integrity test
As a developer, I want a test for `ProxyConn` that writes data in both directions and confirms no bytes are dropped or reordered.

**Acceptance criteria:**
- Two `net.Pipe()` pairs simulate the remote side and local side.
- Known byte sequences are written from each side.
- Reads from the opposite side return the same sequences.
- `ProxyConn` goroutine exits cleanly after both sides close.
