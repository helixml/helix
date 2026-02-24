# Design: Testing RevDial Connectivity

## Architecture Overview

RevDial enables bi-directional communication between the Helix API and sandbox containers:

```
Browser → Helix API → RevDial Dialer → WebSocket → RevDial Listener → Sandbox HTTP Server
```

**Key Components:**
- `api/pkg/revdial/revdial.go` - Server-side Dialer that accepts connections from sandboxes
- `api/pkg/revdial/client.go` - Client-side Listener that runs in sandboxes
- `api/pkg/connman/` - Manages active RevDial connections by device ID
- `api/pkg/hydra/` - Hydra daemon starts RevDial client on sandbox boot

## Existing Test Infrastructure

### CLI Commands (Already Implemented)
1. **`helix spectask screenshot <session-id>`** - Quick RevDial health check
2. **`helix spectask test --session <id> --desktop`** - Full desktop test suite including screenshot
3. **`helix spectask benchmark <session-id>`** - FPS/latency testing (requires active session)

### Integration Tests
- `helix/integration-test/smoke/spectask_mcp_test.go` - E2E MCP tests via RevDial

## Key Design Decisions

### Decision 1: Use Existing Screenshot Endpoint for Testing
**Choice:** Use `/api/v1/external-agents/{session}/screenshot` as the primary RevDial health check.

**Rationale:** 
- Already implemented and working
- Tests full path: API → RevDial → Sandbox HTTP → Screenshot capture → Response
- Fast (~200-500ms typical response time)

### Decision 2: No New Test Infrastructure Needed
**Choice:** Existing `helix spectask test --desktop` and `helix spectask screenshot` commands are sufficient.

**Rationale:**
- Screenshot test already validates RevDial connectivity
- JSON output (`--json`) already supported for CI
- Adding more complexity doesn't improve coverage

## Testing Workflow

### Manual Testing
```bash
# Start a session
helix spectask start --project $HELIX_PROJECT -n "revdial-test"

# Wait for sandbox to connect (~15-20 seconds)
sleep 20

# Test RevDial connectivity
helix spectask screenshot ses_xxx

# Or run full desktop test suite
helix spectask test --session ses_xxx --desktop
```

### CI Testing
```bash
helix spectask test --session ses_xxx --desktop --json --timeout 60
```

## Observability

**Existing Prometheus Metrics:**
- `device_connection_count` - Number of active RevDial connections

**Logs to Check:**
- Sandbox: `/tmp/revdial-client.log` - Shows `✅ RevDial control connection established`
- API: `docker compose logs api | grep -i revdial`

## Failure Modes

| Symptom | Likely Cause | How to Debug |
|---------|--------------|--------------|
| "Sandbox not connected" | RevDial client not started or disconnected | Check sandbox logs for RevDial errors |
| Timeout on screenshot | Network issue or sandbox overloaded | Check sandbox CPU/memory, try `benchmark` |
| 401 Unauthorized | Token expired or invalid | Verify `HELIX_API_KEY` is correct |