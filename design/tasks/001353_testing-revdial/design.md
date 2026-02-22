# Design: Testing RevDial Connectivity

## Architecture Overview

RevDial implements a reverse-dial pattern where sandbox containers initiate outbound WebSocket connections to the Helix API, then the API can "dial back" through this tunnel to reach services inside the container.

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│  Helix API      │◄────────│   RevDial       │◄────────│  Sandbox        │
│  (server.go)    │ Control │   Tunnel        │ Outbound│  Container      │
│                 │  Conn   │                 │  WS     │  (Hydra)        │
├─────────────────┤         └─────────────────┘         ├─────────────────┤
│  connman        │                                     │  desktop-bridge │
│  (Dialer pool)  │─────────── Data Conn ──────────────►│  :9876          │
└─────────────────┘                                     └─────────────────┘
```

### Key Components

| Component | File | Role |
|-----------|------|------|
| `revdial.Dialer` | `api/pkg/revdial/revdial.go` | Server-side: creates connections back to client |
| `revdial.Listener` | `api/pkg/revdial/revdial.go` | Client-side: accepts incoming connections |
| `revdial.Client` | `api/pkg/revdial/client.go` | Reusable client wrapper with reconnect logic |
| `connman` | `api/pkg/connman/connman.go` | Connection manager with grace period support |

## Testing Strategy

### 1. Unit Tests (api/pkg/revdial/)

**New file: `revdial_test.go`**

Test the core protocol without network:
- `TestExtractHostAndTLS` - URL parsing for various schemes
- `TestControlMessage` - JSON serialization of control messages
- `TestDialerRegistration` - Dialer map registration/unregistration

These tests use mock `net.Conn` implementations (similar to `connman_test.go`).

### 2. Integration Tests

**Existing connman tests already cover grace period behavior.** 

For full RevDial testing, use the existing CLI approach:
```bash
helix spectask screenshot <session-id>
```

This validates the complete path: API → connman → RevDial → container → desktop-bridge.

## Key Design Decisions

### Decision 1: No Mock WebSocket Server
**Choice:** Use real connections in integration tests, mock `net.Conn` for unit tests.
**Rationale:** RevDial's WebSocket upgrade logic is complex. Mocking it provides false confidence. The existing `spectask screenshot` CLI already serves as an E2E test.

### Decision 2: Focus Unit Tests on Parsing/Serialization
**Choice:** Unit tests focus on `ExtractHostAndTLS()` and control message handling.
**Rationale:** These are pure functions easy to test. The connection management is already well-tested in `connman_test.go`.

### Decision 3: Reuse Existing CLI for Connectivity Tests
**Choice:** Use `helix spectask screenshot` as the connectivity validation tool.
**Rationale:** It already exists, tests the full path, and provides useful output (screenshot file).

## Codebase Patterns

- **Test suites**: Use `testify/suite` for setup/teardown (see `connman_test.go`)
- **Mock connections**: Implement `net.Conn` interface with channels for control
- **No external dependencies**: Unit tests should not require running services