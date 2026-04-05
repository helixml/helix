# Implementation Tasks

## Protocol

- [ ] Add `cancel_current_turn` command and `turn_cancelled` event to `zed-4/crates/external_websocket_sync/PROTOCOL_SPEC.md`

## Zed (`zed-4`)

- [ ] Add `cancel_current_turn` handler arm in `websocket_sync.rs` command match block (`~line 400`)
- [ ] Implement cancellation: look up active ACP task by `request_id`, abort it, send `turn_cancelled` event back to Helix
- [ ] Handle no-op case: if `request_id` not found or turn already done, send `turn_cancelled` with `status: noop`

## Helix Backend (`helix-4`)

- [ ] Add `interrupted` to `InteractionState` enum in `api/pkg/types/types.go`
- [ ] Add `sendCancelToExternalAgent(sessionID, requestID string)` helper in `websocket_external_agent_sync.go` that sends `cancel_current_turn` command
- [ ] Add handler for incoming `turn_cancelled` event in `websocket_external_agent_sync.go` — resolve pending cancel and mark the interaction `interrupted`
- [ ] Update `processInterruptPrompt()` in `prompt_history_handlers.go` to: (1) find current waiting interaction, (2) call `sendCancelToExternalAgent` with 3s timeout, (3) mark interaction interrupted, (4) then send new `chat_message`

## Helix Backend: Cancel Endpoint

- [ ] Add `POST /api/v1/sessions/{id}/cancel` endpoint that finds the current `waiting` interaction, calls `sendCancelToExternalAgent`, and returns 202

## Helix Frontend (`helix-4`)

- [ ] Add a "Cancel" button in `RobustPromptInput.tsx` next to the send button, visible only when the session has a `waiting` interaction
- [ ] Cancel button calls `POST /api/v1/sessions/{id}/cancel` and shows spinner until the interaction state updates to `interrupted`
- [ ] Render interactions with `interrupted` state with an "Interrupted" label (not spinner, not error icon) in the chat UI
