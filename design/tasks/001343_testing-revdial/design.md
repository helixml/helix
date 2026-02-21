# Design: Testing RevDial Connectivity

## Context

The `api/pkg/revdial` package implements a reverse-dial pattern where sandboxes (Hydra daemon) establish outbound WebSocket connections to the API server, allowing the server to initiate requests to sandboxes behind NAT/firewalls.

**Key Components:**
- `Client` - Runs in sandbox, connects to API via WebSocket, proxies requests to local service
- `Listener` - Accepts connections from a `Dialer` via control messages
- `Dialer` - Server-side component that requests new connections from clients
- `ConnHandler` - HTTP handler for data connections

## Architecture

```
┌─────────────────────┐         ┌─────────────────────┐
│  Sandbox (Client)   │ ──WS──▶ │  API (Dialer)       │
│  - RevDial Client   │         │  - RevDial Handler  │
│  - Local HTTP Svc   │ ◀─────  │  - ConnHandler      │
└─────────────────────┘  proxy  └─────────────────────┘
```

## Test Strategy

### Unit Tests for `revdial_test.go`

1. **TestExtractHostAndTLS** - Table-driven tests for URL parsing
   - HTTP URLs with/without port
   - HTTPS URLs with/without port  
   - WS/WSS URLs
   - Edge cases (trailing slashes, paths)

2. **TestDialLocal** - Connection helper tests
   - TCP address format validation
   - Unix socket path extraction
   - Connection timeout behavior (use short timeout)

3. **TestClientConfig** - Configuration validation
   - Default reconnect delay (1 second)
   - Required fields (ServerURL, RunnerToken)

### Integration-style Tests for `client_test.go`

4. **TestClientStartStop** - Lifecycle tests
   - Client starts without panic when not configured
   - Client stops cleanly via context cancellation

5. **TestListenerDialerRoundtrip** - Core functionality
   - Use `httptest.Server` with WebSocket upgrader
   - Verify control message exchange (keep-alive, conn-ready)
   - Verify data connection establishment

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Use `httptest.Server` for WebSocket mocking | Avoids external dependencies, fast tests |
| Table-driven tests for URL parsing | Covers many edge cases concisely |
| Skip reconnection timing tests | Flaky; focus on behavior not timing |
| Don't mock `net.Conn` deeply | Test real behavior with loopback |

## Dependencies

- `github.com/gorilla/websocket` (already in use)
- `github.com/stretchr/testify/assert` (existing test pattern)
- Standard library `httptest`, `net`

## Files to Create/Modify

| File | Action |
|------|--------|
| `api/pkg/revdial/revdial_test.go` | Create - tests for `Dialer`, `Listener`, `ConnHandler` |
| `api/pkg/revdial/client_test.go` | Create - tests for `Client` and helper functions |