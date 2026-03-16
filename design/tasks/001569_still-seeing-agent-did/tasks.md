# Implementation Tasks

## Bug 1: "[Agent did not respond]" shown after streaming completes

- [ ] Add diagnostic logging: in `handleMessageCompleted` log whether `messageRequestID` is empty or populated, and in `updateCommentWithStreamingResponse` log when `GetCommentByRequestID` fails (check if the error is being swallowed at the call site)
- [ ] Investigate the call site of `updateCommentWithStreamingResponse` in `websocket_external_agent_sync.go` to confirm errors are being logged, not silently dropped
- [ ] Confirm whether the agent includes `request_id` in the `message_completed` WebSocket payload (check Zed/agent side)
- [ ] Fix whichever failure mode is confirmed: ensure streaming updates persist to DB AND `finalizeCommentResponse` is reliably called with the correct `requestID`
- [ ] Test end-to-end: send a comment, watch it stream in, confirm after completion that `comment.agent_response` in the DB matches the streamed content and `request_id` is cleared

## Bug 2: Cmd+C / Ctrl+C does not copy text

- [ ] In `DesignReviewContent.tsx` `handleKeyPress`, add `if (!e.ctrlKey && !e.metaKey)` guard around the `case "c"` shortcut body so Ctrl+C and Cmd+C are not intercepted
- [ ] Test that bare `c` still toggles the comment form and that Ctrl+C / Cmd+C copy selected text normally
