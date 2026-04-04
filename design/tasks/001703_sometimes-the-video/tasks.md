# Implementation Tasks

## Phase 1: Diagnostics

- [ ] Add enhanced close event logging in `websocket-stream.ts` (code, reason, wasClean, time since open)
- [ ] Add `lastOpenTime` tracking to measure connection duration before close
- [ ] Log `reconnectAttempts` value on each close/reconnect event
- [ ] Add connection state machine logging in `DesktopStreamViewer.tsx`
- [ ] Verify close codes are propagated from backend to frontend

## Phase 2: Connection Stability Fix

- [ ] Add connection stability timer in `websocket-stream.ts` - only reset `reconnectAttempts` after 2s stable connection
- [ ] Clear stability timer on close to prevent race conditions
- [ ] Add `isReconnecting` guard to prevent concurrent reconnection attempts
- [ ] Test rapid disconnect/reconnect scenarios

## Phase 3: Backend Close Code Propagation

- [ ] Update `resilient.go` to send WebSocket close frame with code before terminating
- [ ] Pass close reason from proxy errors to client connection
- [ ] Update `ws_stream.go` to log specific close reasons

## Phase 4: Verification

- [ ] Test manual network disconnect/reconnect (should stabilize in 1-2 attempts)
- [ ] Test rapid toggle (should not create infinite loop)
- [ ] Test 30+ minute streaming session (no spurious reconnections)
- [ ] Verify Stats for Nerds shows meaningful close codes
