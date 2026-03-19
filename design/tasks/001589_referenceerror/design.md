# Design: Fix ReferenceError TypesInteractionState is not defined

## Root Cause

**File:** `frontend/src/contexts/streaming.tsx` line 21

The `interaction_update` handler (optimized path for Zed/external agent streaming) references `TypesInteractionState.InteractionStateComplete` at two locations (lines 347 and 383), but the enum is not included in the import from `../api/api`.

Vite uses esbuild for TypeScript transpilation (no type-checking), so the missing import compiles fine but fails at runtime when the handler runs on the `InteractionStateComplete` transition.

## Fix

Add `TypesInteractionState` to the existing import in `streaming.tsx`:

```typescript
// frontend/src/contexts/streaming.tsx line 21
import { TypesInteraction, TypesInteractionState, TypesMessage, TypesSession } from "../api/api";
```

No other changes needed. The enum is already defined and exported from `frontend/src/api/api.ts` (auto-generated via `./stack update_openapi`).

## Why TypeScript Didn't Catch This

Vite's default TypeScript handling uses `esbuild` in `transpileOnly` mode — it strips types and compiles syntax but does NOT run `tsc` type-checking. Missing imports for values (not just types) silently pass the build. Only a full `tsc --noEmit` or `yarn build` with type-checking enabled would have caught this.

## Patterns Learned

- This project uses auto-generated TypeScript API client at `frontend/src/api/api.ts` (regenerated via `./stack update_openapi`)
- All frontend types like `TypesInteractionState`, `TypesSession`, `TypesInteraction` come from this generated file
- Vite does NOT type-check during build — missing value imports can slip through
- The `interaction_update` WebSocket path in `streaming.tsx` was added as an optimization for Zed agent streaming; it's distinct from the older `session_update` path
