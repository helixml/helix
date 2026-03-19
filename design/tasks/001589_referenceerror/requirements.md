# Requirements: Fix ReferenceError TypesInteractionState is not defined

## Problem

A fatal `ReferenceError: TypesInteractionState is not defined` occurs in production for users viewing session/task pages. The error crashes the WebSocket message handler that processes streaming interaction updates.

## Root Cause

`streaming.tsx` uses `TypesInteractionState.InteractionStateComplete` at lines 347 and 383, but the import at line 21 does not include `TypesInteractionState`. Vite transpiles TypeScript without type-checking, so the missing import passes the build but fails at runtime.

```typescript
// Current (broken):
import { TypesInteraction, TypesMessage, TypesSession } from "../api/api";

// Required:
import { TypesInteraction, TypesInteractionState, TypesMessage, TypesSession } from "../api/api";
```

## User Stories

- **As a user**, when I have an active session and the agent completes an interaction, the UI should update correctly without crashing.

## Acceptance Criteria

- [ ] `TypesInteractionState` is imported in `streaming.tsx`
- [ ] No runtime `ReferenceError` occurs in production when an interaction reaches `InteractionStateComplete`
- [ ] Frontend builds without errors (`yarn build`)
- [ ] The interaction completion path works end-to-end in the browser
