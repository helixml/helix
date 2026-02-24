# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial system that allows sandbox containers (behind NAT) to establish outbound connections to the Helix API, which can then dial back into them. This enables the API to reach services inside sandboxes (screenshot, input, clipboard, etc.) without direct inbound access.

## User Stories

### US-1: Developer verifies RevDial is working
As a developer, I want to quickly test that RevDial connectivity is working for a session, so I can debug connection issues.

**Acceptance Criteria:**
- Can run `helix spectask screenshot <session-id>` to verify end-to-end connectivity
- Command returns screenshot data or clear error message
- Timeout after 15 seconds with actionable error

### US-2: Developer diagnoses connection failures
As a developer, I want to understand why RevDial connections fail, so I can fix the underlying issue.

**Acceptance Criteria:**
- Clear error messages distinguish between: no connection, authentication failure, timeout, proxy error
- Logs show connection state transitions (connecting → connected → disconnected)
- `connman` logs show grace period and reconnection attempts

### US-3: CI validates RevDial pipeline
As a CI system, I want to run automated RevDial tests, so I can catch regressions in the proxy pipeline.

**Acceptance Criteria:**
- `helix spectask test --session <id> --desktop` runs screenshot + window list tests
- JSON output available via `--json` flag for CI parsing
- Non-zero exit code on any failure

## Functional Requirements

1. **Screenshot endpoint** (`GET /api/v1/external-agents/{session}/screenshot`) must proxy through RevDial to desktop-bridge
2. **Connection manager** must handle reconnections within 30-second grace period
3. **WebSocket control channel** must send keep-alive pings every 15-18 seconds
4. **Authentication** must validate bearer token before establishing RevDial connection

## Non-Functional Requirements

- Screenshot response time < 5 seconds under normal conditions
- RevDial reconnection should complete within 5 seconds
- No connection leaks (Prometheus metric `device_connection_count` should remain stable)