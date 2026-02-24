# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial tunneling system that allows sandbox containers (behind NAT) to establish outbound WebSocket connections to the Helix API, enabling the API to initiate requests back to the sandboxes. Testing this connectivity is currently manual and ad-hoc.

## User Stories

### US1: Developer verifies RevDial is working
As a developer, I want to quickly verify that RevDial connectivity is working between the API and sandbox containers, so I can debug connection issues.

**Acceptance Criteria:**
- [ ] Can run a CLI command to test RevDial connectivity for a given sandbox/session
- [ ] Command shows clear success/failure status
- [ ] On failure, shows diagnostic info (connection state, error messages)

### US2: CI validates RevDial in integration tests
As a CI pipeline, I want automated tests that verify RevDial works end-to-end, so regressions are caught before merge.

**Acceptance Criteria:**
- [ ] Unit tests exist for `revdial` package (client + server)
- [ ] Integration test establishes real RevDial tunnel and sends data through it
- [ ] Tests run in under 30 seconds

### US3: Operator monitors RevDial health
As an operator, I want visibility into RevDial connection health, so I can diagnose production issues.

**Acceptance Criteria:**
- [ ] Prometheus metrics for active RevDial connections (already exists: `DeviceConnectionCount`)
- [ ] API endpoint to list connected sandboxes with RevDial status
- [ ] Log messages on connect/disconnect with sandbox ID

## Out of Scope

- Performance benchmarking of RevDial throughput
- TLS certificate management for RevDial
- Multi-region RevDial topology