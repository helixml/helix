# Design: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandbox containers (behind NAT/firewalls) to establish connections back to the Helix API server. This enables the API to initiate requests to sandboxes without direct network access.

## Architecture

```
┌─────────────┐         ┌─────────────┐         ┌─────────────────┐
│  Helix API  │◄───────►│   RevDial   │◄───────►│ Sandbox (Hydra) │
│   Server    │ Control │   connman   │  Data   │  behind NAT     │
└─────────────┘   WS    └─────────────┘  proxy  └─────────────────┘
```

**Components:**
- `api/pkg/revdial/` - Core RevDial protocol (Dialer + Listener)
- `api/pkg/connman/` - Connection manager with grace period handling
- `api/cmd/hydra/` - Sandbox agent that establishes RevDial connections
- `api/pkg/cli/spectask/` - CLI tools including `screenshot` command for testing

## Key Design Decisions

### 1. Use Existing CLI Infrastructure

The `spectask screenshot` command already tests RevDial connectivity end-to-end:
- Browser → API → RevDial → Sandbox → Screenshot → Response

No new tools needed—just document and enhance existing patterns.

### 2. Test Levels

| Level | What it tests | How |
|-------|---------------|-----|
| Unit | connman grace period, reconnection | `go test ./api/pkg/connman/` |
| Integration | Full RevDial path | `helix spectask screenshot <session>` |
| E2E | Multiple concurrent sessions | `helix spectask test --desktop --session <id>` |

### 3. Failure Modes to Test

1. **Connection not established** - Sandbox not connected yet
2. **Connection dropped** - Sandbox disconnected mid-request
3. **Grace period recovery** - Reconnection within 30s window
4. **Timeout** - Sandbox slow to respond

## Testing Strategy

### Quick Connectivity Check

```bash
# Start session, wait for sandbox, test screenshot
helix spectask start --project $HELIX_PROJECT -n "revdial-test"
sleep 20
helix spectask screenshot <session-id>
```

### Comprehensive Test

```bash
# Uses existing test infrastructure
helix spectask test --session <id> --desktop
```

Tests: screenshot, list_windows, get_workspaces (all via RevDial).

### Connection Manager Unit Tests

Already exist in `api/pkg/connman/connman_test.go`:
- Grace period behavior
- Reconnection handling
- Context cancellation
- Max pending dials

## Non-Goals

- New testing framework (use existing `spectask test`)
- Mock RevDial server (real integration tests preferred)
- Automated CI pipeline changes (manual testing sufficient for now)