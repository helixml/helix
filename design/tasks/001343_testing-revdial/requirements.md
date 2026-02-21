# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandboxes (behind NAT/firewalls) to initiate WebSocket connections to the Helix control plane, enabling bidirectional communication for screenshots, input, clipboard, and video streaming.

Currently, there are no unit tests for the `api/pkg/revdial` package. This task adds tests to ensure RevDial connectivity works correctly.

## User Stories

### US1: Developer validates RevDial client/server communication
As a developer, I want unit tests for the RevDial package so that I can catch regressions when modifying the connectivity code.

**Acceptance Criteria:**
- [ ] Tests verify `Client` can establish WebSocket connection to server
- [ ] Tests verify `Listener` can accept connections from `Dialer`
- [ ] Tests verify bidirectional data proxying works correctly
- [ ] Tests run without external dependencies (mock WebSocket server)

### US2: Developer validates reconnection behavior
As a developer, I want tests for auto-reconnect logic so that I can ensure sandboxes recover from network issues.

**Acceptance Criteria:**
- [ ] Tests verify client reconnects after connection drop
- [ ] Tests verify configurable reconnect delay is respected

### US3: Developer validates helper functions
As a developer, I want tests for utility functions so that edge cases are covered.

**Acceptance Criteria:**
- [ ] Tests for `ExtractHostAndTLS()` with http/https/ws/wss URLs
- [ ] Tests for `DialLocal()` with TCP and Unix socket addresses
- [ ] Tests verify proper TLS detection from URL scheme

## Out of Scope

- E2E integration tests (already exist via `spectask screenshot` CLI)
- Load testing / stress testing
- Changes to RevDial protocol itself