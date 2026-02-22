# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandboxed containers (behind NAT) to establish connections back to the Helix API server. This enables the API to reach services running inside containers (screenshots, clipboard, input events) without direct network access.

## User Stories

### US1: Developer validates RevDial connection
As a developer, I want to run unit tests for the RevDial package so that I can verify the core protocol logic works correctly without needing a full stack.

### US2: Operator verifies sandbox connectivity
As an operator, I want to use CLI commands to test RevDial connectivity to a running sandbox session so that I can diagnose connection issues.

### US3: CI validates RevDial end-to-end
As a CI system, I want to run integration tests that verify RevDial proxying works end-to-end so that regressions are caught before deployment.

## Acceptance Criteria

### AC1: Unit tests for revdial package
- [ ] Tests for `ExtractHostAndTLS()` function (URL parsing)
- [ ] Tests for `Dialer` creation and message serialization
- [ ] Tests for `Listener` creation and control message handling
- [ ] Tests cover error cases (connection failures, timeouts)

### AC2: CLI connectivity test
- [ ] `helix spectask screenshot <session-id>` validates RevDial by fetching a screenshot
- [ ] Clear error messages when RevDial connection fails
- [ ] Timeout handling (15s default)

### AC3: Integration test coverage
- [ ] Test establishes RevDial control connection (WebSocket)
- [ ] Test creates data connection through RevDial
- [ ] Test proxies HTTP request to container service
- [ ] Test handles reconnection gracefully (connman grace period)

## Out of Scope

- Performance benchmarks for RevDial throughput
- Load testing with multiple concurrent connections
- TLS certificate validation testing