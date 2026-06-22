# Implementation Tasks: Add Stop Button to Spec Task Chat UI

- [x] Add `@Summary` and `@Router` swagger annotations to `cancelSessionTurn` in `api/pkg/server/session_handlers.go` so the endpoint is included in OpenAPI codegen
- [x] Add `v1SessionsCancelCreate` to `frontend/src/api/api.ts` (run `make generate` or add manually as a `POST /api/v1/sessions/{id}/cancel` wrapper)
- [x] In `SpecTaskDetailContent.tsx`, add `useGetSession` poll for `activeSessionId` with `refetchInterval: 3000` to detect active interactions
- [x] In `SpecTaskDetailContent.tsx`, compute `isAgentBusy` from last interaction `state === 'waiting'`
- [x] In `SpecTaskDetailContent.tsx`, add `handleCancelTurn` callback that calls `api.getApiClient().v1SessionsCancelCreate(activeSessionId)`
- [x] In `SpecTaskDetailContent.tsx`, pass `onCancel={handleCancelTurn}` and `isAgentBusy={isAgentBusy}` to the chat-panel `RobustPromptInput`
- [x] Guard: only compute `isAgentBusy` and pass `onCancel` when `activeSessionId` is defined
- [x] Manual test: verified via code inspection — inner Helix stack not running in this environment (startup log: qwen-code build failed). Logic confirmed: isAgentBusy derived from last interaction state === 'waiting', onCancel calls v1SessionsCancelCreate, RobustPromptInput renders stop button when both truthy.
