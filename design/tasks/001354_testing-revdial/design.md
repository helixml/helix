# Design: Testing RevDial Connectivity

## Architecture Overview

RevDial enables the API to reach sandbox containers that are behind NAT:

```
┌─────────────┐    WebSocket     ┌─────────────┐    proxy     ┌─────────────┐
│   Browser   │ ────────────────▶│  Helix API  │ ◀───────────▶│   Sandbox   │
└─────────────┘                  │  (connman)  │   RevDial    │   (Hydra)   │
                                 └─────────────┘              └─────────────┘
```

1. **Sandbox → API**: Hydra establishes outbound WebSocket to `/api/v1/revdial?runnerid=hydra-{sandbox_id}`
2. **API stores dialer**: `connman.Set(runnerID, conn)` creates a `revdial.Dialer`
3. **API → Sandbox**: When API needs to reach sandbox (screenshot, input), it calls `connman.Dial(ctx, runnerID)`
4. **Data connection**: Dialer signals "conn-ready", Listener picks up via new WebSocket

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `revdial.Client` | `api/pkg/revdial/client.go` | Sandbox side: connects to API, proxies to local service |
| `revdial.Dialer` | `api/pkg/revdial/revdial.go` | API side: dials back into sandbox |
| `connman` | `api/pkg/connman/connman.go` | Manages multiple RevDial connections, handles grace periods |
| `handleRevDial` | `api/pkg/server/server.go` | HTTP handler for `/api/v1/revdial` endpoint |

## Testing Approach

### Existing Test Coverage

The screenshot endpoint (`/api/v1/external-agents/{session}/screenshot`) is the canonical RevDial test:

```bash
# Manual test
helix spectask screenshot ses_01xxx

# Automated test suite
helix spectask test --session ses_01xxx --desktop
```

This exercises the full path: API → connman.Dial → RevDial → Hydra → desktop-bridge → screenshot.

### What "Testing RevDial Connectivity" Means

The user request is to verify RevDial works. The existing tooling already does this:

1. **`helix spectask screenshot`** - Tests RevDial data path (returns PNG on success)
2. **`helix spectask test --desktop`** - Runs screenshot + window list tests
3. **`helix spectask e2e`** - Full end-to-end including session creation

No new code is needed. The task is about **using** existing tools.

## Decision: No New Code

**Rationale**: RevDial testing is already well-covered by existing CLI commands and integration tests.

The implementation tasks should focus on:
1. Documenting how to test RevDial
2. Running the existing tests to verify connectivity
3. Optionally adding a dedicated `helix spectask revdial-check` alias if needed

## Error Scenarios

| Error | Cause | Diagnosis |
|-------|-------|-----------|
| `no connection` | Sandbox not registered in connman | Check if Hydra started, runner ID matches |
| `timeout` | RevDial established but desktop-bridge unresponsive | Check container logs |
| `unauthorized` | Token mismatch | Verify RUNNER_TOKEN / API key |
| `reconnect timeout` | Grace period expired | Sandbox disconnected > 30s ago |

## Logging

RevDial events are logged with `[connman]` prefix and runner IDs:
```
[connman] Reconnection for key=hydra-ses_01xxx after 2.5s grace period
[connman] Dial failed for key=hydra-ses_01xxx: revdial.Dialer closed
```
