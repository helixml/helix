# Requirements: Testing RevDial Connectivity

## Overview

RevDial is Helix's reverse-dial mechanism that allows the API server to initiate connections to sandbox containers behind NAT. Testing RevDial connectivity is critical for diagnosing issues where the API cannot communicate with running sandbox sessions.

## User Stories

### US1: Developer tests RevDial from CLI
As a developer, I want to quickly test if RevDial connectivity is working for a specific session, so I can diagnose sandbox communication issues.

**Acceptance Criteria:**
- CLI command tests RevDial connection establishment
- Reports connection status (connected/disconnected/grace period)
- Shows latency metrics for the connection
- Works with existing `helix spectask` CLI

### US2: Automated tests verify RevDial health
As a CI system, I want to verify RevDial connectivity as part of integration tests, so I catch regressions in the sandbox communication layer.

**Acceptance Criteria:**
- Test can be run via `go test` with build tags
- Verifies control connection establishment
- Verifies data connection (via screenshot or exec endpoint)
- Reports clear pass/fail with diagnostic info

### US3: Operator checks RevDial status via API
As an operator, I want to query RevDial connection status for all sandboxes, so I can monitor system health.

**Acceptance Criteria:**
- API endpoint returns connection manager stats
- Lists active connections and keys
- Shows grace period entries (recently disconnected)

## Out of Scope
- RevDial protocol changes
- New connection management logic
- Performance benchmarking (use existing `spectask benchmark` for that)