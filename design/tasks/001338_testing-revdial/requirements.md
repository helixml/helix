# Testing RevDial Connectivity - Requirements

## Overview

Add unit tests for the `revdial` package to ensure the reverse-dial connectivity system works correctly. RevDial allows NAT'd sandbox containers to connect out to the API server, enabling the API to dial back into sandboxes for screenshots, input injection, and video streaming.

## User Stories

### US-1: Developer Confidence
As a developer, I want unit tests for the revdial package so I can refactor or modify RevDial code with confidence that I haven't broken connectivity.

### US-2: CI Validation
As a maintainer, I want RevDial tests to run in CI so connectivity regressions are caught before merge.

## Acceptance Criteria

### AC-1: Unit Tests for Client
- [ ] Test `ExtractHostAndTLS()` correctly parses URLs (http/https/ws/wss, with/without ports)
- [ ] Test `DialLocal()` handles TCP and Unix socket addresses
- [ ] Test client configuration validation (missing ServerURL, missing token)

### AC-2: Unit Tests for Dialer/Listener
- [ ] Test `NewDialer()` creates dialer with correct pickup path
- [ ] Test `Dialer.Done()` channel closes when dialer is closed
- [ ] Test `controlMsg` JSON serialization/deserialization
- [ ] Test `newUniqID()` generates unique 32-character hex strings

### AC-3: Test Coverage
- [ ] New tests must pass: `go test ./api/pkg/revdial/...`
- [ ] No test flakiness (deterministic, no timing-dependent failures)

## Out of Scope

- Integration tests requiring running API server
- End-to-end tests with real sandbox containers (use existing `spectask` CLI tools for that)
- Performance/load testing