# Design: Testing RevDial Connectivity

## Context

RevDial is Helix's reverse-dial tunneling system. Sandbox containers initiate outbound WebSocket connections to the API, and the API can then "dial back" through that tunnel to reach services inside the sandbox. This is critical for screenshots, input injection, and video streaming.

**Key files:**
- `api/pkg/revdial/revdial.go` - Core Dialer/Listener implementation
- `api/pkg/revdial/client.go` - Client wrapper with auto-reconnect
- `api/pkg/connman/connman.go` - Connection manager tracking RevDial connections
- `api/pkg/server/server.go` - `/api/v1/revdial` endpoint handler

## Current State

- `connman_test.go` has unit tests for connection manager (grace periods, reconnects)
- No unit tests for `revdial` package itself
- Manual testing via `spectask screenshot` command
- Prometheus metric `DeviceConnectionCount` tracks active connections

## Design Decisions

### Decision 1: Add unit tests to revdial package

**Approach:** Create `revdial_test.go` with tests for:
- Dialer/Listener handshake over in-memory connection
- Control message protocol (keep-alive, conn-ready, pickup-failed)
- Connection timeout and cleanup

**Rationale:** The revdial package has no tests. Unit tests catch regressions in the WebSocket upgrade, message parsing, and connection lifecycle.

### Decision 2: Add CLI connectivity test command

**Approach:** Extend `spectask` CLI with a `test-revdial` subcommand that:
1. Connects to API
2. Requests a ping through RevDial to the sandbox
3. Reports success/latency or failure reason

**Rationale:** Developers already use `spectask` for testing. A dedicated connectivity test is more reliable than inferring status from screenshot success.

### Decision 3: Add health check endpoint

**Approach:** Add `GET /api/v1/sandboxes/{id}/health` that:
1. Checks if sandbox has active RevDial connection
2. Optionally pings through RevDial to verify tunnel is responsive
3. Returns connection metadata (connected_since, last_activity)

**Rationale:** Operators need API-accessible health checks for monitoring dashboards.

## Architecture

```
┌─────────────────┐     WebSocket      ┌──────────────────┐
│  Sandbox (Hydra)│ ───────────────────▶│  API Server      │
│                 │   CONTROL conn     │  /api/v1/revdial │
│  RevDial Client │                    │  RevDial Dialer  │
└────────┬────────┘                    └────────┬─────────┘
         │                                      │
         │  When API needs to reach sandbox:    │
         │  1. Dialer sends "conn-ready"        │
         │  2. Client opens new DATA WebSocket  │
         │  3. Dialer matches, returns conn     │
         ▼                                      ▼
┌─────────────────┐                    ┌──────────────────┐
│  Local Service  │◀───── proxied ─────│  API Handler     │
│  (Hydra HTTP)   │       data         │  (screenshot,etc)│
└─────────────────┘                    └──────────────────┘
```

## Test Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | revdial Dialer/Listener | In-memory net.Pipe connections |
| Unit | control message parsing | JSON marshal/unmarshal |
| Integration | Full tunnel | Start sandbox container, verify screenshot works |
| E2E | CLI test-revdial | Part of `spectask` smoke tests |

## Risks

- **WebSocket upgrade sensitivity:** RevDial uses HTTP hijacking; tests must use real HTTP servers, not mocks
- **Timing-dependent:** Keep-alive and reconnect logic has timeouts; tests need deterministic timing or generous margins