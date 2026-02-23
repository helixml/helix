# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandbox containers behind NAT to expose services (Hydra API) back to the Helix API server. Testing this connectivity is critical for ensuring spec tasks, screenshots, and desktop interactions work correctly.

## User Stories

### US-1: Developer verifies RevDial connection
As a developer, I want to quickly test if RevDial is working for a session so I can debug connectivity issues without guessing.

**Acceptance Criteria:**
- Can run a single CLI command to test RevDial for a session
- Get clear pass/fail output with latency information
- See helpful error messages when connection fails

### US-2: CI validates RevDial in integration tests
As a CI pipeline, I need automated tests that verify RevDial connectivity so regressions are caught before merge.

**Acceptance Criteria:**
- Tests can run headlessly with JSON output
- Exit code reflects pass/fail status
- Tests complete within reasonable timeout (30s default)

### US-3: Operator diagnoses connection problems
As an operator, I want to see the RevDial connection state for active sessions so I can diagnose user-reported issues.

**Acceptance Criteria:**
- Can list all active RevDial connections
- Can see connection age and status
- Can identify sessions in grace period (temporarily disconnected)

## Functional Requirements

1. **Screenshot test** - Validates full path: API → connman → RevDial → Hydra → screenshot
2. **Connection status** - Shows whether a session's sandbox has active RevDial connection
3. **Latency measurement** - Reports round-trip time through RevDial tunnel
4. **Grace period visibility** - Shows sessions with recently-lost connections awaiting reconnect

## Non-Functional Requirements

- Tests timeout gracefully (no hanging)
- Works with both local dev and production environments
- Output is human-readable by default, JSON for scripting