# Requirements: Testing RevDial Connectivity

## Overview

RevDial is Helix's reverse dial mechanism that allows sandbox containers (behind NAT) to establish outbound connections to the API, which can then be used to dial back into the container. Testing RevDial connectivity ensures the control plane can reach sandboxes for screenshots, input injection, and MCP tools.

## User Stories

### US1: Developer validates RevDial connectivity
As a developer, I want to quickly test if RevDial is working for a session, so I can debug connectivity issues.

**Acceptance Criteria:**
- Run `helix spectask screenshot <session-id>` and get a PNG back (or clear error)
- Response time < 5 seconds for a healthy connection
- Clear error messages distinguish: no connection, timeout, auth failure

### US2: CI validates sandbox connectivity
As a CI system, I want automated tests that verify RevDial works end-to-end, so regressions are caught before deploy.

**Acceptance Criteria:**
- `helix spectask test --session <id> --desktop` runs screenshot + window list tests
- Exit code 0 on success, non-zero on failure
- JSON output option (`--json`) for CI parsing

### US3: Operator diagnoses connection issues
As an operator, I want visibility into RevDial connection state, so I can diagnose why a sandbox is unreachable.

**Acceptance Criteria:**
- Connection manager stats visible (active connections, grace period entries)
- Logs show connection/disconnection events with runner IDs
- Grace period behavior documented (30s default reconnect window)

## Technical Constraints

- RevDial uses WebSocket for control channel, HTTP upgrade for data connections
- Connection manager (`connman`) tracks active dialers and handles reconnection grace periods
- Existing `helix spectask screenshot` and `helix spectask test --desktop` already test this path