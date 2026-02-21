# Testing RevDial Connectivity - Design

## Architecture

RevDial is a minimal SOCKS5-like reverse tunnel with two components:

1. **Server side (API)**: `handleRevDial()` in `server.go` accepts WebSocket connections, registers them in `connman`, and uses `revdial.ConnHandler` for data connections.

2. **Client side (Sandbox)**: `revdial.Client` connects to API via WebSocket, creates a `revdial.Listener` that accepts incoming dial requests from the server.

```
Sandbox Container                    API Server
┌─────────────────┐                 ┌─────────────────┐
│ revdial.Client  │ ──WebSocket──► │ handleRevDial() │
│                 │   (control)     │                 │
│ revdial.Listener│ ◄──WebSocket── │ revdial.Dialer  │
│                 │   (data)        │                 │
│ Local Service   │ ◄──TCP/Unix──  │ connman.Dial()  │
└─────────────────┘                 └─────────────────┘
```

## Test Strategy

### Unit Tests (`api/pkg/revdial/revdial_test.go`)

Test pure functions and serialization without network:

1. **`ExtractHostAndTLS()`** - URL parsing logic
2. **`newUniqID()`** - Unique ID generation
3. **`controlMsg` JSON** - Message serialization

### Unit Tests (`api/pkg/revdial/client_test.go`)

Test client helper functions:

1. **`DialLocal()`** - Unix socket vs TCP address detection
2. **Configuration validation** - Ensure missing config is handled

### Existing Coverage

- `connman_test.go` already covers connection manager behavior (grace periods, reconnection, pending dials)
- Integration testing covered by `helix spectask screenshot/stream/keyboard-test` CLI tools

## Key Decisions

1. **Unit tests only** - No spinning up real servers; keeps tests fast and deterministic
2. **Table-driven tests** - Use Go's table-driven pattern for URL parsing variations
3. **No mocking WebSockets** - The Dialer/Listener integration requires real connections; rely on existing CLI tools for integration testing

## File Changes

| File | Change |
|------|--------|
| `api/pkg/revdial/revdial_test.go` | New - Test `newUniqID`, `controlMsg` |
| `api/pkg/revdial/client_test.go` | New - Test `ExtractHostAndTLS`, `DialLocal` |