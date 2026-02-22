# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandbox containers (behind NAT) to establish outbound WebSocket connections to the Helix API, which can then be used to "dial back" into those containers. This is critical for screenshots, input injection, clipboard, and video streaming.

## User Stories

### US1: Developer Connectivity Verification
As a developer, I want to quickly verify that RevDial connectivity is working between the API and a sandbox container, so I can debug connection issues.

**Acceptance Criteria:**
- Can test RevDial connection using existing CLI tools (`spectask screenshot`, `spectask stream`)
- Clear error messages when connection fails (timeout, auth error, no sandbox)
- Latency metrics reported for round-trip time

### US2: Automated Health Checks
As an operator, I want automated health checks that verify RevDial tunnels are functional, so I can monitor production deployments.

**Acceptance Criteria:**
- Health check endpoint returns RevDial connection status per sandbox
- Connection manager stats exposed (active connections, grace period entries)
- Prometheus metrics for connection count and latency

## Existing Test Methods

The following already exist and should be documented:

1. **Screenshot Test**: `helix spectask screenshot <session-id>` - Tests full RevDial path
2. **Stream Test**: `helix spectask stream <session-id>` - Tests WebSocket streaming via RevDial
3. **Benchmark**: `helix spectask benchmark <session-id>` - Measures FPS (uses RevDial)

## Out of Scope

- New RevDial protocol changes
- Unit tests for revdial package (existing code is stable)
- Load testing infrastructure