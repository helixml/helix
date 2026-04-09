# Implementation Tasks

## Change 1: Re-queue failed events (Zed)
- [ ] In `websocket_sync.rs`, add a `VecDeque<SyncEvent>` retry buffer in `reconnection_loop` and pass `&mut` to `run_connection`
- [ ] In `run_connection`, drain the retry buffer before reading from `outgoing_rx` in the select loop
- [ ] When `ws_sink.send()` fails (line 307-312), push the event into the retry buffer instead of dropping it, then return to trigger reconnection
- [ ] Test: kill API mid-stream, verify the in-flight event is delivered on reconnect (check API logs for the message_added/message_completed arriving)

## Change 2: Request-ID deduplication in Zed
- [ ] Add a `COMPLETED_REQUESTS: Lazy<Mutex<HashMap<String, CompletedRequest>>>` static in `thread_service.rs` to track recently completed request_ids (struct holds request_id, acp_thread_id, completed_at)
- [ ] In the `Stopped` event handler (where `message_completed` is sent, ~line 718), record the request_id in `COMPLETED_REQUESTS`
- [ ] Add TTL cleanup: evict entries older than 10 minutes when inserting new ones
- [ ] In the `chat_message` handler, before dispatching work, check if `request_id` is in `COMPLETED_REQUESTS` — if yes, log it and re-send `message_completed`, skip re-processing
- [ ] Test: send a chat_message, let it complete, then send the same chat_message again (same request_id) — verify Zed replies with message_completed without re-invoking Claude Code

## Change 3: Timeout stuck waiting interactions (API)
- [ ] In `pickupWaitingInteraction` (`websocket_external_agent_sync.go`), after queuing the chat_message for the agent, start a goroutine with a 120-second timeout
- [ ] If no `message_completed` arrives for that request_id within the timeout, mark the interaction as `error` state with message "Response interrupted by system restart"
- [ ] Ensure the timeout goroutine is cancelled if `message_completed` does arrive (use a channel or context)
- [ ] Test: restart API with a waiting interaction where Zed never responds — verify it transitions to error state after timeout

## Bump & CI
- [ ] Commit Zed changes, update `sandbox-versions.txt` with new ZED_COMMIT hash
- [ ] Run E2E WebSocket sync tests to verify no regressions
- [ ] Test end-to-end: start a session, send a message, restart the API (`docker compose restart api`), verify the response completes and the chat view updates
