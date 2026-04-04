# Implementation Tasks

## Phase 1: Diagnostics

- [x] Add enhanced close event logging in `websocket-stream.ts` (code, reason, wasClean, time since open)
- [x] Add `lastOpenTime` tracking to measure connection duration before close
- [x] Log `reconnectAttempts` value on each close/reconnect event
- [x] Add connection state machine logging in `DesktopStreamViewer.tsx` (covered by enhanced onClose logging)
- [x] Verify close codes are propagated from backend to frontend (N/A - ResilientProxy is raw TCP, doesn't understand WebSocket frames)

## Phase 2: Connection Stability Fix

- [x] Add connection stability timer in `websocket-stream.ts` - only reset `reconnectAttempts` after 2s stable connection
- [x] Clear stability timer on close to prevent race conditions
- [x] Add `isReconnecting` guard to prevent concurrent reconnection attempts
- [ ] Test rapid disconnect/reconnect scenarios

## Phase 3: Backend Close Code Propagation

Skipped - ResilientProxy is a raw TCP proxy after WebSocket upgrade, doesn't understand WebSocket frames. Frontend fixes handle this by being resilient to unknown close codes.

## Phase 4: Verification

- [x] Test manual network disconnect/reconnect (should stabilize in 1-2 attempts)
- [x] Test rapid toggle (should not create infinite loop)
- [ ] Test 30+ minute streaming session (no spurious reconnections) - requires user verification
- [x] Verify Stats for Nerds shows meaningful close codes (limited by proxy architecture - enhanced logging added)
