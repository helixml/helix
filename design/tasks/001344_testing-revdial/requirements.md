# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial mechanism that allows sandbox containers (behind NAT) to establish tunnels back to the Helix API server. The API can then dial into sandboxes for screenshots, input injection, and other control operations.

**Current state**: No unit tests exist for the RevDial package. Testing is done manually via `helix spectask screenshot <session-id>`.

## User Stories

### US-1: Developer can verify RevDial connectivity in isolation
As a developer, I want unit tests for the RevDial client/server so I can verify the tunnel works without spinning up full sandbox infrastructure.

**Acceptance Criteria:**
- [ ] Unit tests cover `revdial.NewDialer()` and `revdial.NewListener()` communication
- [ ] Tests verify bidirectional data flow through the tunnel
- [ ] Tests cover connection teardown and reconnection scenarios
- [ ] Tests run with `go test ./api/pkg/revdial/...`

### US-2: Developer can test connection manager grace period
As a developer, I want tests for the `connman` package so I can verify the grace period reconnection logic works.

**Acceptance Criteria:**
- [ ] Tests verify pending dials are queued during disconnection
- [ ] Tests verify pending dials complete after reconnection
- [ ] Tests verify grace period expiration cleans up properly
- [ ] Tests cover max pending dials limit

### US-3: CI pipeline validates RevDial on every PR
As a developer, I want RevDial tests in CI so regressions are caught before merge.

**Acceptance Criteria:**
- [ ] Tests pass in Drone CI
- [ ] Tests complete in <30 seconds
- [ ] No external dependencies (no real sandboxes needed)

## Out of Scope

- E2E tests requiring real sandbox containers (use existing `spectask screenshot` for that)
- Performance/load testing
- TLS certificate testing