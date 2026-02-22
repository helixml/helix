# Design: Testing RevDial Connectivity

## Architecture Overview

RevDial creates reverse tunnels from sandbox containers to the API server:

```
┌─────────────────┐          ┌──────────────────┐
│  Sandbox        │          │  Helix API       │
│  (behind NAT)   │          │  (public)        │
│                 │          │                  │
│  RevDial Client ├──────────►  RevDial Server  │
│  (outbound WS)  │  tunnel  │  (accepts conn)  │
│                 │          │                  │
│  Local Service  │◄─────────┤  Dialer.Dial()   │
│  (hydra socket) │  reverse │  (dials back)    │
└─────────────────┘          └──────────────────┘
```

**Key components:**
- `revdial.Listener` - Sandbox-side: accepts commands from API over tunnel
- `revdial.Dialer` - API-side: creates connections to sandbox services via tunnel
- `connman.ConnectionManager` - Manages multiple sandbox connections with grace period reconnection

## Test Strategy

### Unit Tests for `api/pkg/revdial/`

Create `revdial_test.go` with:

1. **TestDialerListenerRoundtrip** - Create in-memory pipe, establish tunnel, send data both directions
2. **TestDialerClose** - Verify `Dialer.Done()` is signaled on close
3. **TestListenerAccept** - Verify listener accepts connections from dialer
4. **TestMultipleConnections** - Open multiple logical connections over single tunnel

Use `net.Pipe()` to simulate network connections without real sockets.

### Unit Tests for `api/pkg/connman/`

Create `connman_test.go` with:

1. **TestSetAndDial** - Basic connection registration and dialing
2. **TestGracePeriodQueue** - Verify pending dials queue during disconnection
3. **TestGracePeriodReconnect** - Verify queued dials complete on reconnect
4. **TestGracePeriodExpiry** - Verify cleanup after grace period expires
5. **TestMaxPendingDials** - Verify limit is enforced

Use short grace periods (100ms) for fast tests.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Test isolation | `net.Pipe()` | No network stack needed, fast, deterministic |
| Grace period in tests | 100ms | Fast tests, still exercises timing logic |
| No mocks for revdial | Real implementation | Simple enough to test directly |
| Separate test files | One per package | Standard Go convention |

## Existing Patterns

The codebase uses `testify/suite` for test organization (see `api/pkg/server/` tests). The revdial tests should follow the same pattern for consistency.

## Files to Create

```
api/pkg/revdial/revdial_test.go     # ~150 lines
api/pkg/connman/connman_test.go     # ~200 lines
```

## Dependencies

- `github.com/stretchr/testify/suite` - Already in go.mod
- `github.com/stretchr/testify/require` - Already in go.mod
- No new dependencies needed