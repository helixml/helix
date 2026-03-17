# Design

## Overview

One-line fix: add `TypesInteractionState` to the existing import in `streaming.tsx`.

## Affected File

`frontend/src/contexts/streaming.tsx`

**Current import (line 21):**
```typescript
import { TypesInteraction, TypesMessage, TypesSession } from "../api/api";
```

**Fixed import:**
```typescript
import { TypesInteraction, TypesInteractionState, TypesMessage, TypesSession } from "../api/api";
```

## Why This Happened

`TypesInteractionState` is a TypeScript `enum` exported from the auto-generated `frontend/src/api/api.ts`. It has four values:

```typescript
export enum TypesInteractionState {
  InteractionStateNone     = "",
  InteractionStateWaiting  = "waiting",
  InteractionStateEditing  = "editing",
  InteractionStateComplete = "complete",
  InteractionStateError    = "error",
}
```

Every other file that uses this enum imports it correctly (`useLiveInteraction.ts`, `Session.tsx`, `Interaction.tsx`, etc.). `streaming.tsx` was the only file that referenced the enum without importing it — likely introduced when the WebSocket handler was refactored or the enum comparison was added.

## Error Locations in streaming.tsx

| Line | Code |
|------|------|
| 347  | `updatedInteraction.state === TypesInteractionState.InteractionStateComplete` |
| 383  | `updatedInteraction.state === TypesInteractionState.InteractionStateComplete` |

Both are inside `handleWebsocketEvent`, which is called by the WebSocket `messageHandler` for every incoming `interaction_update` event — so the error fires on every streaming response completion.

## Pattern Note

This project auto-generates `frontend/src/api/api.ts` from the backend OpenAPI spec. Enums like `TypesInteractionState` are generated there and must be explicitly imported wherever used. TypeScript's `isolatedModules` or bundler tree-shaking can mask missing enum imports during development if the enum value happens to be inlined — always verify enum imports at build/runtime.
