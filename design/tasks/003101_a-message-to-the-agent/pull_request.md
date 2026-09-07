# Wake stopped agents when they receive a message

## Summary
Allow a new chat message to wake an explicitly stopped agent immediately. The prompt queue now reaps a dead waiting interaction without applying the normal stale-interaction delay when the sandbox reports `stopped` or `terminated_idle`, then proceeds through the existing dispatch and sandbox startup path. Live connections and ambiguous mid-boot disconnects retain their existing safeguards.

Update the stopped-sandbox notice to explain that sending a message wakes the agent.

## Testing
Ran `go test ./api/pkg/server -run 'TestPromptHistoryHandlersSuite' -count=1` successfully. Added regression coverage for stopped and idle-terminated sandboxes, transient disconnects, and live WebSocket connections.

Updated the focused frontend assertion for the new notice text. The frontend test could not be executed in the checkout because Vitest dependencies were not installed (`vitest: not found`).
