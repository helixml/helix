# Requirements: WebSocket Session State Sync After API Restart

## Problem Statement

When the Helix API restarts, users lose the ability to receive real-time updates for their connected Zed sessions. Messages are sometimes received much later than expected, and sending a message from Zed seems to "wake up" the connection and deliver pending updates.

## Root Cause

The API uses an embedded NATS server for pub/sub messaging. When the API restarts:
1. The embedded NATS server restarts, losing all subscriptions
2. Frontend WebSocket reconnects successfully (using `ReconnectingWebSocket`)
3. A new NATS subscription is created on reconnect
4. **But**: Messages published during the disconnect window are lost (NATS pub/sub is fire-and-forget)
5. The client remains unaware of session state changes until the next event triggers a publish

## User Stories

### US-1: Session State Recovery After Reconnect
**As a** user with an active Zed session  
**When** my WebSocket connection is re-established after API restart  
**I want** to immediately see the current session state  
**So that** I don't miss any agent activity that occurred during the disconnect

### US-2: No Stale UI State
**As a** user viewing a session in the frontend  
**When** the API restarts and my WebSocket reconnects  
**I want** the UI to reflect the actual session state  
**So that** I'm not confused by outdated information (e.g., "waiting" when agent already responded)

## Acceptance Criteria

### AC-1: Immediate State Sync on Connect
- [ ] When a WebSocket connection is established to `/api/v1/ws/user`, the server immediately sends the current session state
- [ ] The state sync message uses the existing `session_update` event type
- [ ] This works for both initial connections and reconnections

### AC-2: Graceful Handling of Missing Sessions
- [ ] If the session no longer exists, send an appropriate error/empty state
- [ ] Don't crash or hang if session lookup fails

### AC-3: No Duplicate Processing
- [ ] Frontend handles receiving the same session state multiple times idempotently
- [ ] UI doesn't flicker or reset scroll position on redundant updates

### AC-4: Minimal Latency Impact
- [ ] The state sync should not add noticeable delay to WebSocket connection establishment
- [ ] Database query for session state should be efficient (single query)

## Out of Scope

- Persistent message queuing (JetStream) - would be a larger architectural change
- Message sequence numbers for gap detection - future enhancement
- Reconnection of external agent (Zed) WebSocket connections - separate issue