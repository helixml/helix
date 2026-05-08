# Skip "agent finished" notification when user is already active in session

## Summary

When an interaction ends because the user sent a new message — either interrupting the agent mid-response or following up immediately after it finishes — the system was emitting an "Agent finished working" attention event. That notification is just noise: the user is clearly already looking at the session.

This change suppresses the `agent_interaction_completed` attention event (and its downstream Slack post / browser notification) whenever the session already has a newer interaction in `waiting` state by the time `message_completed` is processed.

## Changes

- `api/pkg/server/websocket_external_agent_sync.go` — in `handleMessageCompleted()`, before kicking off the attention-event goroutine, query the latest interaction in the session. If it's `state=waiting` and was created after the just-completed interaction, skip the notification with a debug log.
- `api/pkg/server/websocket_external_agent_sync_test.go` — add `TestMessageCompleted_SkipsAttentionWhenUserActive` and `TestMessageCompleted_EmitsAttentionWhenNoFollowup` covering both paths. Tests verify that `GetSpecTask` (the first store call inside the goroutine) is not called when suppressed and is called when not suppressed.

## Behaviour

- Failure of the suppression query falls through to emit the event as normal (safe default).
- Non-spectask sessions are unaffected — the check sits inside the existing `if helixSession.Metadata.SpecTaskID != ""` block.
