# Implementation Tasks: Add Stop Button to Spec Task Chat UI

- [x] Add `@Summary` and `@Router` swagger annotations to `cancelSessionTurn` in `api/pkg/server/session_handlers.go` so the endpoint is included in OpenAPI codegen
- [x] Add `v1SessionsCancelCreate` to `frontend/src/api/api.ts` (run `make generate` or add manually as a `POST /api/v1/sessions/{id}/cancel` wrapper)
- [x] In `SpecTaskDetailContent.tsx`, add `useGetSession` poll for `activeSessionId` with `refetchInterval: 3000` to detect active interactions
- [x] In `SpecTaskDetailContent.tsx`, compute `isAgentBusy` from last interaction `state === 'waiting'`
- [x] In `SpecTaskDetailContent.tsx`, add `handleCancelTurn` callback that calls `api.getApiClient().v1SessionsCancelCreate(activeSessionId)`
- [x] In `SpecTaskDetailContent.tsx`, pass `onCancel={handleCancelTurn}` and `isAgentBusy={isAgentBusy}` to the chat-panel `RobustPromptInput`
- [x] Guard: only compute `isAgentBusy` and pass `onCancel` when `activeSessionId` is defined
- [~] Manual test: start an agent turn in the spec-task view, verify the Stop button appears; click it, verify the turn is cancelled and the button disappears
