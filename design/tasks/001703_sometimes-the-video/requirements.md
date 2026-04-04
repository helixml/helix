# Requirements: Video Streaming Reconnect Loop Fix

## Problem Statement

Video streaming sometimes gets stuck in a reconnect loop or reconnects successfully but then almost immediately reconnects again. Refreshing the page fixes the issue. The Stats for Nerds panel shows repeated socket disconnections with "error unknown".

## User Stories

### US-1: Stable Video Stream Connection
As a user, I want the video stream to maintain a stable connection without getting stuck in a reconnect loop, so I can work without interruption.

**Acceptance Criteria:**
- [ ] Video stream maintains connection after initial successful connect
- [ ] If reconnection is needed, it completes within 2-3 attempts
- [ ] No rapid repeated reconnections (e.g., reconnect → connect → reconnect within seconds)

### US-2: Clear Error Reporting
As a user, I want to see meaningful error messages when disconnections occur, so I can understand what's happening.

**Acceptance Criteria:**
- [ ] Socket close events include meaningful close codes (not just "unknown")
- [ ] Stats for Nerds shows specific disconnection reasons
- [ ] Connection log shows actionable information

### US-3: Auto-Recovery Without Page Refresh
As a user, I want the streaming to automatically recover from transient issues without requiring a page refresh.

**Acceptance Criteria:**
- [ ] Transient network issues trigger reconnection (not permanent failure)
- [ ] Reconnection uses proper exponential backoff
- [ ] Connection stabilizes after reconnection succeeds

## Technical Context

### Architecture Overview
```
Browser ← WebSocket → API (ResilientProxy) ← RevDial → desktop-bridge ← GStreamer/PipeWire
```

### Key Components
1. **Frontend** (`websocket-stream.ts`): Handles WebSocket connection, reconnection logic
2. **API Proxy** (`resilient.go`): Bidirectional proxy with reconnection support
3. **Desktop Bridge** (`ws_stream.go`): Video streaming via GStreamer/PipeWire

### Current Reconnection Behavior
- Frontend: Exponential backoff (1s base, max 30s, 10 attempts)
- Frontend: Heartbeat stale detection (10s timeout)
- Backend: ResilientProxy reconnects on server errors (3 attempts, 30s timeout)
- Backend: Proxy deduplication (same client_id cancels old proxy)

## Root Cause Hypotheses

1. **reconnectAttempts not properly reset**: The counter may not reset after successful connection, causing premature reconnect abort
2. **Race condition in proxy deduplication**: New connection cancels old before fully established
3. **Stale connection detection false positive**: Heartbeat timer triggers during legitimate slow periods
4. **Multiple event handlers causing duplicate reconnects**: Both WebSocketStream and DesktopStreamViewer may trigger reconnection
