# Requirements

## Problem

Production Sentry error affecting all users using the WebSocket-powered session streaming:

```
Uncaught TypeError: Cannot read properties of undefined (reading 'InteractionStateComplete')
  at WebSocket._handleMessage (index-CZReX8gA.js)
```

## Root Cause

`/home/retro/work/helix/frontend/src/contexts/streaming.tsx` uses `TypesInteractionState.InteractionStateComplete` on lines 347 and 383, but `TypesInteractionState` is **not imported**. At runtime the identifier is `undefined`, so accessing `.InteractionStateComplete` throws.

## User Stories

- As a user chatting in a session, I should not see Sentry error popups caused by WebSocket message handling crashes.
- As a developer, the streaming context should correctly detect when an interaction completes so downstream state (e.g. `patchPendingRef`) is properly reset.

## Acceptance Criteria

- [ ] `TypesInteractionState` is imported in `streaming.tsx` alongside the other types.
- [ ] No runtime TypeError when a WebSocket `interaction_update` event is received with `state === "complete"`.
- [ ] The `patchPendingRef` flag and `isComplete` logic work correctly after fix.
- [ ] No new Sentry errors of this type in production.
