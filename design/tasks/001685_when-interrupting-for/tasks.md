# Implementation Tasks

## Protocol

- [x] Add `cancel_current_turn` command and `turn_cancelled` event to `zed/crates/external_websocket_sync/PROTOCOL_SPEC.md`

## Zed (`zed`)

- [x] Add `cancel_current_turn` handler arm in `websocket_sync.rs` command match block (`~line 400`)
- [x] Implement cancellation: look up active ACP task by `request_id`, abort it, send `turn_cancelled` event back to Helix
- [x] Handle no-op case: if `request_id` not found or turn already done, send `turn_cancelled` with `status: noop`

## Helix Backend (`helix`)

- [x] Add `interrupted` to `InteractionState` enum in `api/pkg/types/types.go`
- [x] Add `sendCancelToExternalAgent(sessionID, requestID string)` helper in `websocket_external_agent_sync.go` that sends `cancel_current_turn` command
- [x] Add handler for incoming `turn_cancelled` event in `websocket_external_agent_sync.go` — resolve pending cancel and mark the interaction `interrupted`
- [x] Update `processInterruptPrompt()` in `prompt_history_handlers.go` to: (1) find current waiting interaction, (2) call `sendCancelToExternalAgent` with 3s timeout, (3) mark interaction interrupted, (4) then send new `chat_message`

## Helix Backend: Cancel Endpoint

- [x] Add `POST /api/v1/sessions/{id}/cancel` endpoint that finds the current `waiting` interaction, calls `sendCancelToExternalAgent`, and returns 202

## Zed E2E Tests (`zed-4/crates/external_websocket_sync/e2e-test`)

- [ ] Add new E2E phase (Phase 13): Helix-initiated cancel via `cancel_current_turn` — send a `chat_message`, then mid-stream send `cancel_current_turn` with the same `request_id`, assert Zed replies with `turn_cancelled` (status `cancelled`) and stops streaming
- [ ] Add new E2E phase (Phase 14): Cancel no-op — send `cancel_current_turn` with a `request_id` that has already completed (or never existed), assert Zed replies with `turn_cancelled` (status `noop`)
- [ ] Update `roundState` in `main.go` with phase 13/14 tracking fields
- [ ] Update `validateRound()` to check phase 13/14 assertions (turn_cancelled received, streaming stopped, interaction marked interrupted in store)
- [ ] Add `turn_cancelled` event handling to `syncEventCallback()` in the test driver
- [ ] Update Rust `protocol_test.rs` unit test to cover `cancel_current_turn` command handling and `turn_cancelled` event emission

## Helix Frontend (`helix-4`)

- [~] Add a "Cancel" button in `RobustPromptInput.tsx` next to the send button, visible only when the session has a `waiting` interaction
- [ ] Cancel button calls `POST /api/v1/sessions/{id}/cancel` and shows spinner until the interaction state updates to `interrupted`
- [ ] Render interactions with `interrupted` state with an "Interrupted" label (not spinner, not error icon) in the chat UI
