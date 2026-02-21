# Requirements: Testing RevDial Connectivity

## Overview

RevDial is a reverse-dial WebSocket tunnel that allows Helix sandboxes (behind NAT) to accept incoming connections from the control plane. Testing RevDial connectivity is critical for validating that screenshots, clipboard, input injection, and video streaming work correctly.

## User Stories

### US-1: Developer verifies RevDial connection
**As a** developer debugging sandbox connectivity  
**I want to** quickly test if RevDial is working  
**So that** I can isolate network issues from application bugs

**Acceptance Criteria:**
- [ ] Can check if sandbox has an active RevDial connection to control plane
- [ ] Get clear success/failure indication with error details
- [ ] Test completes within 10 seconds

### US-2: Developer tests end-to-end screenshot flow
**As a** developer  
**I want to** take a screenshot through the RevDial tunnel  
**So that** I can verify the full control plane → sandbox → response path works

**Acceptance Criteria:**
- [ ] Screenshot request routes through RevDial to sandbox
- [ ] Returns valid JPEG image data
- [ ] Fails with clear error if RevDial is disconnected

### US-3: CI pipeline validates RevDial health
**As a** CI system  
**I want to** run automated RevDial connectivity tests  
**So that** regressions are caught before deployment

**Acceptance Criteria:**
- [ ] Tests can run in headless/non-interactive mode
- [ ] Exit codes indicate pass/fail
- [ ] Output is machine-parseable (JSON option)

## Functional Requirements

### FR-1: Connection Status Check
- Query RevDial connection status for a given sandbox/session
- Report: connected/disconnected, runner ID, connection duration

### FR-2: Screenshot Test
- Use existing `helix spectask screenshot <session-id>` command
- Validates: RevDial routing, Hydra socket, screenshot-server

### FR-3: Round-trip Latency Measurement
- Measure time from request to response through RevDial tunnel
- Useful for diagnosing performance issues

## Non-Functional Requirements

- Tests should work with existing CLI (`helix spectask`)
- No new infrastructure required—use existing endpoints
- Support both local dev and production environments