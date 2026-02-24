# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandbox containers (behind NAT) to establish connections back to the Helix API, enabling the API to proxy requests into the sandbox. This task focuses on testing RevDial connectivity to ensure reliable communication between the API and sandbox containers.

## User Stories

### US-1: Developer tests RevDial connection manually
**As a** developer  
**I want to** verify RevDial connectivity for a running session  
**So that** I can diagnose connection issues between the API and sandbox

**Acceptance Criteria:**
- [ ] Can run `helix spectask screenshot <session-id>` to test the RevDial tunnel
- [ ] Clear error messages when RevDial is not connected
- [ ] Success message confirms the tunnel is working

### US-2: CI validates RevDial in integration tests
**As a** CI system  
**I want to** run automated RevDial connectivity tests  
**So that** we catch regressions in the RevDial infrastructure

**Acceptance Criteria:**
- [ ] `helix spectask test --session <id> --desktop` tests screenshot endpoint
- [ ] JSON output mode (`--json`) for CI parsing
- [ ] Tests complete within configurable timeout (default 30s)

### US-3: Operator monitors RevDial health
**As an** operator  
**I want to** see which sandboxes have active RevDial connections  
**So that** I can identify disconnected or unhealthy sandboxes

**Acceptance Criteria:**
- [ ] API exposes connected sandbox count via Prometheus metrics
- [ ] Can query which sandbox IDs are currently connected

## What Exists Today

- `helix spectask screenshot <session-id>` - Tests screenshot endpoint via RevDial
- `helix spectask test --desktop` - Runs desktop MCP tests including screenshot
- `api/pkg/revdial/` - Core RevDial client and server implementation
- `api/pkg/connman/` - Connection manager tracking RevDial dialers
- Prometheus metric `device_connection_count` tracks connected devices

## Out of Scope

- Load testing multiple concurrent RevDial connections
- Modifying the RevDial protocol itself
- Adding new RevDial endpoints