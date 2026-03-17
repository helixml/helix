# Implementation Tasks

- [ ] In `frontend/src/contexts/streaming.tsx` line 21, add `TypesInteractionState` to the import from `"../api/api"`:
  ```typescript
  import { TypesInteraction, TypesInteractionState, TypesMessage, TypesSession } from "../api/api";
  ```
- [ ] Verify the frontend builds without TypeScript errors (`npm run build` or equivalent)
- [ ] Manually test a session chat to confirm WebSocket `interaction_update` events complete without console errors
