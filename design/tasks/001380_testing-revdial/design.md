# Design: Testing RevDial Connectivity

## Context

RevDial (reverse dial) enables the Helix API to initiate connections to sandbox containers that are behind NAT. The architecture:

1. **Control Connection**: Sandbox (Hydra) establishes WebSocket to API (`/api/v1/revdial?runnerid=xxx`)
2. **Data Connections**: When API needs to reach sandbox, it sends `conn-ready` message, sandbox dials back
3. **Connection Manager**: `api/pkg/connman/` tracks dialers, handles grace periods for reconnection

Current testing is ad-hoc via `helix spectask screenshot` which implicitly tests RevDial by fetching a screenshot through the proxy chain.

## Design Decisions

### D1: Extend existing `spectask` CLI (not new command)

**Decision**: Add `helix spectask revdial <session-id>` subcommand to existing CLI.

**Rationale**: 
- Keeps all sandbox diagnostic tools in one place
- Reuses existing auth, API URL, and session discovery logic
- Consistent with `spectask screenshot`, `spectask benchmark`, etc.

### D2: Test via screenshot endpoint (not raw socket)

**Decision**: Use the existing screenshot endpoint (`/api/v1/external-agents/{session}/screenshot`) as the test payload.

**Rationale**:
- Screenshot exercises the full RevDial path: API → connman → revdial.Dialer → sandbox
- Already implemented and working
- Returns meaningful data (can verify image bytes received)
- Avoids adding new test-only endpoints

### D3: Add connection status API endpoint

**Decision**: Add `GET /api/v1/admin/revdial/status` endpoint returning `connman.Stats()`.

**Rationale**:
- Operators need visibility into connection state
- `connman.Stats()` already exists, just needs HTTP exposure
- Admin-only endpoint (requires admin token)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    helix spectask revdial                       │
│                                                                 │
│  1. GET /api/v1/admin/revdial/status                           │
│     → Check if sandbox has active connection                    │
│                                                                 │
│  2. GET /api/v1/external-agents/{session}/screenshot           │
│     → Test data path through RevDial                           │
│                                                                 │
│  3. Report: connection state, latency, bytes received          │
└─────────────────────────────────────────────────────────────────┘
```

## Key Files

| File | Change |
|------|--------|
| `api/pkg/cli/spectask/revdial_cmd.go` | New CLI command |
| `api/pkg/server/admin_handlers.go` | New status endpoint |
| `integration-test/smoke/revdial_test.go` | Integration test |

## API Endpoint

```
GET /api/v1/admin/revdial/status

Response:
{
  "active_connections": 5,
  "grace_period_entries": 1,
  "pending_dials_total": 0,
  "connections": ["hydra-sandbox-123", "hydra-sandbox-456", ...]
}
```

## CLI Output

```
$ helix spectask revdial ses_01abc123
Testing RevDial connectivity for session ses_01abc123...

Connection Status:
  Sandbox ID:     sandbox-xyz
  RevDial Key:    hydra-sandbox-xyz  
  Status:         ✅ Connected

Data Path Test (screenshot):
  Latency:        45ms
  Bytes received: 125,432
  Status:         ✅ Success

RevDial connectivity: PASSED
```

## Error Cases

- **No connection**: `❌ No RevDial connection for sandbox (not in connman)`
- **Grace period**: `⚠️ Connection in grace period, waiting for reconnect...`
- **Screenshot failed**: `❌ Data path test failed: <error>`
