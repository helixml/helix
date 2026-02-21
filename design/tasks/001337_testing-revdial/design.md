# Design: Testing RevDial Connectivity

## Architecture Overview

RevDial enables sandboxes to receive connections from the control plane despite being behind NAT:

```
Browser → API Server → RevDial Tunnel → Sandbox (Hydra) → screenshot-server/desktop-bridge
                ↑                              ↑
         /api/v1/revdial              unix:///tmp/hydra.sock
```

**Key components:**
- **RevDial Client** (`api/pkg/revdial/client.go`): Runs inside Hydra, maintains WebSocket to API
- **RevDial Server** (`api/pkg/revdial/revdial.go`): API-side listener, routes requests to connected sandboxes
- **Hydra**: Container orchestrator in sandbox, exposes Unix socket for local services

## Existing Test Infrastructure

The `helix spectask` CLI already provides RevDial testing capabilities:

| Command | What it tests |
|---------|---------------|
| `spectask start --project <id>` | Creates session, waits for RevDial-connected sandbox |
| `spectask screenshot <session-id>` | Full round-trip: API → RevDial → Hydra → screenshot-server |
| `spectask benchmark <session-id>` | Video streaming FPS through RevDial |

## Design Decision: Use Existing CLI

**Decision:** Leverage existing `helix spectask` commands rather than creating new test infrastructure.

**Rationale:**
- `spectask screenshot` already validates the full RevDial path
- No code changes required—this is a testing/documentation task
- The CLI handles auth, timeouts, and error reporting

## Test Scenarios

### 1. Basic Connectivity Test
```bash
# Start session and verify sandbox connects
helix spectask start --project $HELIX_PROJECT -n "revdial-test"
# If successful, sandbox has working RevDial connection
```

### 2. Screenshot Round-trip Test
```bash
helix spectask screenshot ses_xxx
# Success = RevDial is working
# Failure = Check: sandbox logs, Hydra status, API logs
```

### 3. Connection Recovery Test
```bash
# Restart API, verify sandbox reconnects
docker compose -f docker-compose.dev.yaml restart api
sleep 10
helix spectask screenshot ses_xxx  # Should work after reconnect
```

## Debugging RevDial Issues

### Check sandbox-side connection
```bash
docker compose exec -T sandbox-nvidia docker logs <ubuntu-container> 2>&1 | grep -i revdial
```

### Check API-side connections
```bash
docker compose -f docker-compose.dev.yaml logs api | grep -i revdial
```

### Expected log patterns
- Good: `✅ RevDial control connection established`
- Bad: `RevDial connection error`, `failed to connect control WebSocket`

## Constraints

- RevDial requires `HELIX_API_URL` and `RUNNER_TOKEN` environment variables
- Local dev uses `docker-compose.dev.yaml`—never the prod compose file
- Screenshot test requires an active session with running desktop container