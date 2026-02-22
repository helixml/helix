# Design: Testing RevDial Connectivity

## Architecture Overview

RevDial enables the Helix API to reach sandbox containers behind NAT:

```
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│  Helix API      │◄────────│  RevDial Tunnel │◄────────│  Sandbox        │
│  (connman)      │  dial   │  (WebSocket)    │ outbound│  (Hydra)        │
└─────────────────┘         └─────────────────┘         └─────────────────┘
```

1. **Sandbox → API**: Container establishes outbound WebSocket to `/api/v1/revdial`
2. **API → Sandbox**: API uses `connman.Dial()` to create logical connections back through the tunnel
3. **Proxy**: RevDial proxies requests to local services (Hydra socket, screenshot server)

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `revdial.Client` | `api/pkg/revdial/client.go` | Client-side (sandbox), establishes tunnel |
| `revdial.Dialer` | `api/pkg/revdial/revdial.go` | Server-side, creates connections through tunnel |
| `connman` | `api/pkg/connman/connman.go` | Manages multiple RevDial connections with grace periods |
| `handleRevDial` | `api/pkg/server/server.go` | HTTP handler accepting RevDial WebSocket connections |

## Testing Approach

### Method 1: Screenshot Endpoint (Recommended)

The screenshot endpoint (`/api/v1/external-agents/{sessionID}/screenshot`) exercises the full RevDial path:

```bash
helix spectask screenshot ses_xxx
```

This tests:
- WebSocket tunnel establishment
- `connman.Dial()` to sandbox
- HTTP request proxied through RevDial
- Response returned through tunnel

### Method 2: Stream Connection

```bash
helix spectask stream ses_xxx --duration 10
```

Tests sustained WebSocket connection through RevDial for video streaming.

### Method 3: Connection Manager Stats

The `connman.Stats()` method provides:
- `ActiveConnections`: Number of live RevDial tunnels
- `GracePeriodEntries`: Disconnected but recoverable connections
- `PendingDialsTotal`: Blocked dial requests waiting for reconnection

## Failure Modes

| Symptom | Likely Cause | Debug Command |
|---------|--------------|---------------|
| "no connection" | Sandbox not connected | Check sandbox logs for RevDial startup |
| Timeout on screenshot | Tunnel dead but not detected | Check `connman.Stats()` |
| Auth error 401 | Invalid runner token | Verify `RUNNER_TOKEN` env var |
| Connection refused | Hydra socket not ready | Check Hydra is listening on socket |

## Existing Metrics

RevDial already exposes Prometheus metrics via `promutil.DeviceConnectionCount` which tracks active dialer registrations.