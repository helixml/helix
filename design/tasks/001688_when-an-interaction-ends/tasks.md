# Implementation Tasks

- [ ] In `handleMessageCompleted()` (`websocket_external_agent_sync.go`), after marking the interaction complete and before the `attentionService.EmitEvent` call, query the store for all interactions in the session
- [ ] If any interaction has `State == InteractionStateWaiting` and `Created` after the completed interaction's `Created`, skip the `EmitEvent` call (log at debug level)
- [ ] If the store query fails, fall through to emit the event as normal (safe default)
- [ ] Write a unit/integration test: completing an interaction when a newer waiting interaction exists → no attention event emitted
- [ ] Write a unit/integration test: completing an interaction with no follow-up → attention event emitted as normal
